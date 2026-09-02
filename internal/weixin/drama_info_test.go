package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestGetDramaSuccess(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok-123" {
			t.Errorf("access_token = %q, want tok-123", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req getDramaRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.DramaID != 10001 {
			t.Errorf("drama_id = %d, want 10001", req.DramaID)
		}

		fmt.Fprint(w, `{
			"errcode": 0,
			"errmsg": "ok",
			"drama_info": {
				"drama_id": 10001,
				"create_time": 1682214878,
				"name": "我的演艺",
				"status": 0,
				"media_count": 2,
				"media_list": [{"media_id": 1}, {"media_id": 2}],
				"audit_detail": {"status": 3, "create_time": 1682215878, "audit_time": 1682235878},
				"actor_list": {"actor": [{"name": "演员1", "role": "角色1"}]}
			}
		}`)
	})

	info, err := client.GetDrama(context.Background(), "tok-123", 10001)
	if err != nil {
		t.Fatalf("GetDrama() error = %v", err)
	}
	if info.DramaID != 10001 || info.Name != "我的演艺" || info.Status != 0 {
		t.Fatalf("GetDrama() = %+v, unexpected core fields", info)
	}
	if len(info.MediaList) != 2 || info.MediaList[0].MediaID != 1 {
		t.Fatalf("MediaList = %+v, want 2 entries starting at media_id 1", info.MediaList)
	}
	if info.AuditDetail.Status != 3 {
		t.Fatalf("AuditDetail.Status = %d, want 3 (approved)", info.AuditDetail.Status)
	}
	if len(info.ActorList.Actor) != 1 || info.ActorList.Actor[0].Name != "演员1" {
		t.Fatalf("ActorList = %+v, unexpected", info.ActorList)
	}
}

func TestGetDramaAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":10093030,"errmsg":"资源不存在"}`)
	})

	_, err := client.GetDrama(context.Background(), "tok-123", 999999)
	if err == nil {
		t.Fatalf("GetDrama() error = nil, want APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("GetDrama() error type = %T, want *APIError", err)
	}
	if apiErr.Code != 10093030 {
		t.Fatalf("APIError.Code = %d, want 10093030", apiErr.Code)
	}
}
