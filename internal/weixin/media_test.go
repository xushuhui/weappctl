package weixin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestDeleteMediaSuccess(t *testing.T) {
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
		var req deleteMediaRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if req.MediaID != 28918028 {
			t.Errorf("media_id = %d, want 28918028", req.MediaID)
		}

		fmt.Fprint(w, `{"errcode":0,"errmsg":"ok"}`)
	})

	if err := client.DeleteMedia(context.Background(), "tok-123", 28918028); err != nil {
		t.Fatalf("DeleteMedia() error = %v", err)
	}
}

func TestDeleteMediaAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":10093030,"errmsg":"资源不存在"}`)
	})

	err := client.DeleteMedia(context.Background(), "tok-123", 999999)
	if err == nil {
		t.Fatalf("DeleteMedia() error = nil, want APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("DeleteMedia() error type = %T, want *APIError", err)
	}
	if apiErr.Code != 10093030 {
		t.Fatalf("APIError.Code = %d, want 10093030", apiErr.Code)
	}
}
