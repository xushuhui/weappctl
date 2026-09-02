package weixin

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestTokenManagerFetchesAndCaches(t *testing.T) {
	var fetches int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fetches++
		fmt.Fprint(w, `{"access_token":"tok-fresh","expires_in":7200,"errcode":0,"errmsg":"ok"}`)
	})

	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tm := &TokenManager{
		Client: client,
		Cache:  NewTokenCache(t.TempDir()),
		AppID:  "wx-appid",
		Secret: "shh",
		Now:    func() time.Time { return now },
	}

	token, err := tm.AccessToken(context.Background(), "default")
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if token != "tok-fresh" {
		t.Fatalf("AccessToken() = %q, want tok-fresh", token)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}

	// A second call, clock unchanged, must be served from cache.
	token, err = tm.AccessToken(context.Background(), "default")
	if err != nil {
		t.Fatalf("AccessToken() (cached) error = %v", err)
	}
	if token != "tok-fresh" || fetches != 1 {
		t.Fatalf("AccessToken() (cached) = %q, fetches = %d; want tok-fresh, 1 (no re-fetch)", token, fetches)
	}
}

func TestTokenManagerRefetchesNearExpiry(t *testing.T) {
	var fetches int
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fetches++
		fmt.Fprint(w, `{"access_token":"tok-2","expires_in":7200,"errcode":0,"errmsg":"ok"}`)
	})

	cache := NewTokenCache(t.TempDir())
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	// Pre-seed a token that expires 30s from "start" — inside refreshBuffer.
	if err := cache.Save("default", &CachedToken{AccessToken: "tok-stale", ExpiresAt: start.Add(30 * time.Second)}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	tm := &TokenManager{
		Client: client,
		Cache:  cache,
		AppID:  "wx-appid",
		Secret: "shh",
		Now:    func() time.Time { return start },
	}

	token, err := tm.AccessToken(context.Background(), "default")
	if err != nil {
		t.Fatalf("AccessToken() error = %v", err)
	}
	if token != "tok-2" {
		t.Fatalf("AccessToken() = %q, want tok-2 (should have refetched near-expiry token)", token)
	}
	if fetches != 1 {
		t.Fatalf("fetches = %d, want 1", fetches)
	}
}

func TestTokenManagerRequiresCredentialsOnCacheMiss(t *testing.T) {
	tm := &TokenManager{
		Client: NewClient(),
		Cache:  NewTokenCache(t.TempDir()),
		Now:    func() time.Time { return time.Now() },
	}

	if _, err := tm.AccessToken(context.Background(), "default"); err == nil {
		t.Fatalf("AccessToken() error = nil, want error for missing appid/secret")
	}
}
