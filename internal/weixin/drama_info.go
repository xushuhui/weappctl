package weixin

import (
	"context"
	"net/http"
	"net/url"
)

const getDramaPath = "/wxa/sec/vod/getdrama"

// DramaInfo is the submission record for one Drama (see CONTEXT.md): a
// title submitted via "剧目提审", independent of whether it is currently
// published. Status reports where it stands in that lifecycle.
type DramaInfo struct {
	// DramaID is documented as a JSON number on this endpoint, unlike
	// PublishedDrama.DramaID (a JSON string on developerGetPublishedDrama).
	// ASSUMPTION: WeChat's own docs disagree on the wire type between these
	// two endpoint families; each struct here matches its own doc verbatim
	// rather than being unified, since guessing wrong would break decoding.
	DramaID           int64        `json:"drama_id"`
	CreateTime        int64        `json:"create_time"`
	Name              string       `json:"name"`
	CoverURL          string       `json:"cover_url"`
	MediaCount        int          `json:"media_count"`
	Producer          string       `json:"producer"`
	Playwright        string       `json:"playwright"`
	Description       string       `json:"description"`
	ProductionLicense string       `json:"production_license"`
	AuditDetail       AuditDetail  `json:"audit_detail"`
	MediaList         []DramaMedia `json:"media_list"`
	// Expedited is 1 when expedited review was requested, 0/absent otherwise.
	Expedited       int       `json:"expedited"`
	Recommendations string    `json:"recommendations"`
	PromotionPoster string    `json:"promotion_poster"`
	ActorList       ActorList `json:"actor_list"`
	// Status: 0 live, 1 in review, 2 review failed, 3 taken down by platform,
	// per WeChat's own docs. VERIFIED DISCREPANCY (against a real account's
	// 1150-drama listDramas dump): Status==1 here does NOT reliably mean
	// "actually in review" — of 167 records with Status==1, only 2 had
	// AuditDetail.Status==1 (in review); the other 165 had
	// AuditDetail.Status==4 (returned for revision, i.e. rejected pending
	// resubmission). The WeChat console's own "审核中" count matches
	// AuditDetail.Status==1, not this field. Treat AuditDetail.Status as
	// the authoritative review-state signal; this Status is closer to a
	// coarse playability flag.
	Status int `json:"status"`
}

// AuditDetail is the review status attached to a DramaInfo.
type AuditDetail struct {
	// Status: 0 invalid, 1 in review, 2 finally rejected, 3 approved, 4
	// returned for revision.
	Status int `json:"status"`
	// AuditType is only populated by the event-push interface, not this
	// endpoint: 0 first submission, 1 resubmission, 2 episode replacement,
	// 3 basic-info edit.
	AuditType  int   `json:"audit_type"`
	CreateTime int64 `json:"create_time"`
	AuditTime  int64 `json:"audit_time"`
}

// DramaMedia identifies one episode's uploaded media asset.
type DramaMedia struct {
	MediaID int64 `json:"media_id"`
}

// ActorList wraps the cast list, matching the API's nested envelope shape.
type ActorList struct {
	Actor []Actor `json:"actor"`
}

// Actor is one cast member entry.
type Actor struct {
	Name            string `json:"name"`
	PhotoMaterialID string `json:"photo_material_id"`
	Role            string `json:"role"`
	Profile         string `json:"profile"`
}

type getDramaRequest struct {
	DramaID int64 `json:"drama_id"`
}

type getDramaResponse struct {
	apiError
	DramaInfo DramaInfo `json:"drama_info"`
}

// GetDrama calls getDrama, returning the submission record for dramaID.
func (c *Client) GetDrama(ctx context.Context, accessToken string, dramaID int64) (*DramaInfo, error) {
	q := url.Values{}
	q.Set("access_token", accessToken)

	var resp getDramaResponse
	if err := c.do(ctx, http.MethodPost, getDramaPath, q, getDramaRequest{DramaID: dramaID}, &resp); err != nil {
		return nil, err
	}
	return &resp.DramaInfo, nil
}
