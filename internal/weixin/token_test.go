package weixin

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestFetchAccessTokenSuccess(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("appid"); got != "wx-appid" {
			t.Errorf("appid = %q, want wx-appid", got)
		}
		if got := r.URL.Query().Get("secret"); got != "shh" {
			t.Errorf("secret = %q, want shh", got)
		}
		// GET calls must not gain the POST-only "{}" default body.
		if ct := r.Header.Get("Content-Type"); ct != "" {
			t.Errorf("Content-Type = %q, want empty (no body on GET)", ct)
		}
		fmt.Fprint(w, `{"access_token":"tok-123","expires_in":7200,"errcode":0,"errmsg":"ok"}`)
	})

	token, expiresIn, err := client.FetchAccessToken(context.Background(), "wx-appid", "shh")
	if err != nil {
		t.Fatalf("FetchAccessToken() error = %v", err)
	}
	if token != "tok-123" || expiresIn != 7200 {
		t.Fatalf("FetchAccessToken() = %q, %d; want tok-123, 7200", token, expiresIn)
	}
}

func TestFetchAccessTokenAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":40013,"errmsg":"invalid appid"}`)
	})

	_, _, err := client.FetchAccessToken(context.Background(), "bad-appid", "shh")
	if err == nil {
		t.Fatalf("FetchAccessToken() error = nil, want APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("FetchAccessToken() error type = %T, want *APIError", err)
	}
	if apiErr.Code != 40013 {
		t.Fatalf("APIError.Code = %d, want 40013", apiErr.Code)
	}
}
