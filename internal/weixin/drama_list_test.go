package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestListDramasSuccess(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var req listDramasRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.Offset != 10 || req.Limit != 50 {
			t.Errorf("request = %+v, want offset=10 limit=50", req)
		}

		fmt.Fprint(w, `{
			"errcode": 0,
			"errmsg": "ok",
			"drama_info_list": [
				{"drama_id": 1, "name": "剧目一", "status": 0},
				{"drama_id": 2, "name": "剧目二", "status": 1}
			]
		}`)
	})

	dramas, err := client.ListDramas(context.Background(), "tok-123", 10, 50)
	if err != nil {
		t.Fatalf("ListDramas() error = %v", err)
	}
	if len(dramas) != 2 || dramas[0].DramaID != 1 || dramas[1].Name != "剧目二" {
		t.Fatalf("ListDramas() = %+v, unexpected", dramas)
	}
}

func TestListDramasAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":47003,"errmsg":"参数不符合要求"}`)
	})

	_, err := client.ListDramas(context.Background(), "tok-123", 0, 200)
	if err == nil {
		t.Fatalf("ListDramas() error = nil, want APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("ListDramas() error type = %T, want *APIError", err)
	}
	if apiErr.Code != 47003 {
		t.Fatalf("APIError.Code = %d, want 47003", apiErr.Code)
	}
}
