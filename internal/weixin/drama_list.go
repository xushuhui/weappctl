package weixin

import (
	"context"
	"net/http"
	"net/url"
)

const listDramasPath = "/wxa/sec/vod/listdramas"

type listDramasRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

type listDramasResponse struct {
	apiError
	DramaInfoList []DramaInfo `json:"drama_info_list"`
}

// ListDramas calls listDramas, returning one page (starting at offset, at
// most limit entries — WeChat caps limit at 100) of every Drama submitted
// under accessToken's account, regardless of review or publish status.
func (c *Client) ListDramas(ctx context.Context, accessToken string, offset, limit int) ([]DramaInfo, error) {
	q := url.Values{}
	q.Set("access_token", accessToken)

	var resp listDramasResponse
	body := listDramasRequest{Offset: offset, Limit: limit}
	if err := c.do(ctx, http.MethodPost, listDramasPath, q, body, &resp); err != nil {
		return nil, err
	}
	return resp.DramaInfoList, nil
}
