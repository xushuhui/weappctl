package weixin

import (
	"context"
	"net/http"
	"net/url"
)

const publishedDramaPath = "/wxadrama/developergetpublisheddrama"

// PublishedDrama is one entry returned by GetPublishedDramas: a Published
// Drama (see CONTEXT.md) — a drama that has cleared "短剧上架设置" review
// and is currently live.
type PublishedDrama struct {
	// SrcAppID is the Submitting Miniprogram's appid (see CONTEXT.md); it
	// is not necessarily the caller's own appid.
	SrcAppID string `json:"src_appid"`
	DramaID  string `json:"drama_id"`
	// PublishTime is a second-precision Unix timestamp.
	PublishTime int64 `json:"publish_time"`
}

type publishedDramaResponse struct {
	apiError
	List []PublishedDrama `json:"list"`
}

// GetPublishedDramas calls developerGetPublishedDrama, returning every
// Published Drama visible to accessToken's miniprogram account.
func (c *Client) GetPublishedDramas(ctx context.Context, accessToken string) ([]PublishedDrama, error) {
	q := url.Values{}
	q.Set("access_token", accessToken)

	var resp publishedDramaResponse
	if err := c.do(ctx, http.MethodPost, publishedDramaPath, q, nil, &resp); err != nil {
		return nil, err
	}
	return resp.List, nil
}
