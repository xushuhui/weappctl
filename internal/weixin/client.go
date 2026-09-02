// Package weixin is a minimal client for the WeChat miniprogram server-side
// APIs weappctl wraps.
package weixin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultBaseURL = "https://api.weixin.qq.com"

// Client calls the WeChat HTTP API. The zero value is not usable; construct
// with NewClient.
type Client struct {
	// BaseURL is exported so tests can point it at an httptest.Server.
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient returns a Client configured for the production WeChat API.
func NewClient() *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// apiError is embedded in every response envelope: every WeChat endpoint
// used here reports success/failure via these two fields.
type apiError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// Err returns a non-nil error when the envelope reports failure.
func (e *apiError) Err() error {
	if e.ErrCode != 0 {
		return &APIError{Code: e.ErrCode, Message: e.ErrMsg}
	}
	return nil
}

// APIError is returned when WeChat's response envelope carries a non-zero
// errcode.
type APIError struct {
	Code    int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("weixin api error %d: %s", e.Code, e.Message)
}

// envelope is satisfied by every response struct via the embedded apiError.
type envelope interface {
	Err() error
}

// do issues an HTTP request against path with the given query parameters.
// When body is non-nil it is JSON-encoded as the request payload. Some
// WeChat POST endpoints reject a genuinely empty body with errcode 44002
// ("POST内容为空") even when their docs say the request payload is empty
// (developerGetPublishedDrama is one), so a nil body on a POST defaults to
// an empty JSON object instead of sending nothing.
// Content-Type: application/json is set whenever a body is sent.
// The response is decoded into out, and the envelope error (if any) returned.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body interface{}, out envelope) error {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var reqBody io.Reader
	sendsBody := body != nil || method == http.MethodPost
	if sendsBody {
		data := []byte("{}")
		if body != nil {
			var err error
			data, err = json.Marshal(body)
			if err != nil {
				return fmt.Errorf("encode %s request: %w", path, err)
			}
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if sendsBody {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("call %s: unexpected status %s", path, resp.Status)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return out.Err()
}
