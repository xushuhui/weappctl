package weixin

import (
	"context"
	"time"
)

// refreshBuffer forces a refetch this long before the cached token would
// actually expire, so a token never goes stale mid-request.
const refreshBuffer = 60 * time.Second

// TokenManager resolves a usable access_token for one profile: it serves
// from TokenCache when the cached token is still fresh, and otherwise
// exchanges AppID/Secret for a new one via Client and re-caches it.
type TokenManager struct {
	Client *Client
	Cache  *TokenCache
	AppID  string
	Secret string

	// Now is injectable for tests; defaults to time.Now when nil.
	Now func() time.Time
}

func (m *TokenManager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

// AccessToken returns a valid access_token for key (the profile name),
// fetching and caching a new one if necessary.
func (m *TokenManager) AccessToken(ctx context.Context, key string) (string, error) {
	now := m.now()

	if cached := m.Cache.Load(key); cached != nil && cached.ExpiresAt.After(now.Add(refreshBuffer)) {
		return cached.AccessToken, nil
	}

	if m.AppID == "" || m.Secret == "" {
		return "", errNoAppIDOrSecret
	}

	token, expiresIn, err := m.Client.FetchAccessToken(ctx, m.AppID, m.Secret)
	if err != nil {
		return "", err
	}

	cached := &CachedToken{
		AccessToken: token,
		ExpiresAt:   now.Add(time.Duration(expiresIn) * time.Second),
	}
	if err := m.Cache.Save(key, cached); err != nil {
		return "", err
	}

	return token, nil
}
