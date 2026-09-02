package weixin

import (
	"context"
	"net/http"
	"net/url"
)

const tokenPath = "/cgi-bin/token"

type tokenResponse struct {
	apiError
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// FetchAccessToken exchanges an appid/secret pair for a fresh access_token,
// valid for expiresIn seconds. It always calls the network; callers wanting
// caching should go through TokenManager instead.
func (c *Client) FetchAccessToken(ctx context.Context, appid, secret string) (accessToken string, expiresIn int, err error) {
	q := url.Values{}
	q.Set("grant_type", "client_credential")
	q.Set("appid", appid)
	q.Set("secret", secret)

	var resp tokenResponse
	if err := c.do(ctx, http.MethodGet, tokenPath, q, nil, &resp); err != nil {
		return "", 0, err
	}
	return resp.AccessToken, resp.ExpiresIn, nil
}
