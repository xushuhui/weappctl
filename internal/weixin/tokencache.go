package weixin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CachedToken is what TokenCache persists: an access_token plus the instant
// it stops being valid.
type CachedToken struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// TokenCache stores one CachedToken per key (profile name) as a plain JSON
// file under dir. See docs/adr/0001-cache-access-token-to-local-file.md for
// why this is a file instead of an in-memory-only or keychain-backed cache.
type TokenCache struct {
	dir string
}

// NewTokenCache returns a TokenCache rooted at dir. dir is created on first
// Save, not here.
func NewTokenCache(dir string) *TokenCache {
	return &TokenCache{dir: dir}
}

func (c *TokenCache) path(key string) string {
	return filepath.Join(c.dir, key+".token.json")
}

// Load returns the cached token for key, or nil if there is none on disk.
// A corrupt cache file is treated as absent rather than a hard error, since
// it is always safe to re-fetch.
func (c *TokenCache) Load(key string) *CachedToken {
	data, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil
	}
	var tok CachedToken
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil
	}
	return &tok
}

// Save persists tok under key, creating the cache directory if needed. The
// file is written 0600 since it holds a live API credential.
func (c *TokenCache) Save(key string, tok *CachedToken) error {
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("create token cache dir %s: %w", c.dir, err)
	}
	data, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("encode cached token: %w", err)
	}
	if err := os.WriteFile(c.path(key), data, 0o600); err != nil {
		return fmt.Errorf("write cached token: %w", err)
	}
	return nil
}

var errNoAppIDOrSecret = errors.New("appid/secret required to fetch an access token")
