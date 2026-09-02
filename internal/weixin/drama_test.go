package weixin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestGetPublishedDramasSuccess(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.URL.Query().Get("access_token"); got != "tok-123" {
			t.Errorf("access_token = %q, want tok-123", got)
		}
		// WeChat rejects a genuinely empty POST body on this endpoint
		// (errcode 44002) despite the docs saying there is no payload;
		// lock in that we always send "{}".
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != "{}" {
			t.Errorf("body = %q, want {}", body)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		fmt.Fprint(w, `{
			"list":[
				{"src_appid":"wx-src-1","drama_id":"drama-1","publish_time":1744626100},
				{"src_appid":"wx-src-2","drama_id":"drama-2","publish_time":1744626200}
			],
			"errcode":0,
			"errmsg":"ok"
		}`)
	})

	dramas, err := client.GetPublishedDramas(context.Background(), "tok-123")
	if err != nil {
		t.Fatalf("GetPublishedDramas() error = %v", err)
	}
	if len(dramas) != 2 {
		t.Fatalf("len(dramas) = %d, want 2", len(dramas))
	}
	want := PublishedDrama{SrcAppID: "wx-src-1", DramaID: "drama-1", PublishTime: 1744626100}
	if dramas[0] != want {
		t.Fatalf("dramas[0] = %+v, want %+v", dramas[0], want)
	}
}

func TestGetPublishedDramasAPIError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"errcode":21000,"errmsg":"该剧未授权播放，请通过后台api进行授权"}`)
	})

	_, err := client.GetPublishedDramas(context.Background(), "tok-123")
	if err == nil {
		t.Fatalf("GetPublishedDramas() error = nil, want APIError")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("GetPublishedDramas() error type = %T, want *APIError", err)
	}
	if apiErr.Code != 21000 {
		t.Fatalf("APIError.Code = %d, want 21000", apiErr.Code)
	}
}
