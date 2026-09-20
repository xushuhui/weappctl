package weixin

import (
	"context"
	"net/http"
	"net/url"
)

const deleteMediaPath = "/wxa/sec/vod/deletemedia"

type deleteMediaRequest struct {
	MediaID int64 `json:"media_id"`
}

type deleteMediaResponse struct {
	apiError
}

// DeleteMedia calls deleteMedia, permanently removing the media asset
// identified by mediaID (see CONTEXT.md's Media entry). This is
// irreversible; weappctl performs no client-side confirmation before
// issuing the call (see docs/adr for the rationale).
func (c *Client) DeleteMedia(ctx context.Context, accessToken string, mediaID int64) error {
	q := url.Values{}
	q.Set("access_token", accessToken)

	var resp deleteMediaResponse
	return c.do(ctx, http.MethodPost, deleteMediaPath, q, deleteMediaRequest{MediaID: mediaID}, &resp)
}
