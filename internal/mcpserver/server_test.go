package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/xsh/weappctl/internal/dramaquery"
	"github.com/xsh/weappctl/internal/weixin"
)

const testToken = "tok-alice"

// fixtureDramas covers the two axes and the overlap the domain model warns
// about: id 2 is approved AND taken down.
func fixtureDramas() []weixin.DramaInfo {
	return []weixin.DramaInfo{
		drama(1, "正常可播", 3, 0),
		drama(2, "通过但下架", 3, 3),
		drama(3, "退回修改", 4, 1),
		drama(4, "审核中", 1, 1),
	}
}

func drama(id int64, name string, audit, status int) weixin.DramaInfo {
	return weixin.DramaInfo{
		DramaID:     id,
		Name:        name,
		Status:      status,
		AuditDetail: weixin.AuditDetail{Status: audit},
	}
}

// weixinStub stands in for api.weixin.qq.com.
func weixinStub(t *testing.T) *httptest.Server {
	t.Helper()
	dramas := fixtureDramas()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/wxa/sec/vod/listdramas":
			fmt.Fprintf(w, `{"drama_info_list":%s}`, mustJSON(t, dramas))
		case "/wxadrama/developergetpublisheddrama":
			fmt.Fprint(w, `{"list":[{"src_appid":"wx1","drama_id":"1","publish_time":1700000000},`+
				`{"src_appid":"wx1","drama_id":"2","publish_time":1700000001},`+
				`{"src_appid":"wx2","drama_id":"3","publish_time":1700000002}]}`)
		case "/wxa/sec/vod/getdrama":
			var body struct {
				DramaID int64 `json:"drama_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode getdrama request: %v", err)
			}
			for _, d := range dramas {
				if d.DramaID == body.DramaID {
					fmt.Fprintf(w, `{"drama_info":%s}`, mustJSON(t, d))
					return
				}
			}
			fmt.Fprint(w, `{"errcode":10090001,"errmsg":"drama not found"}`)
		default:
			t.Errorf("unexpected WeChat path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(data)
}

func newServer(t *testing.T, dailyLimit int) *httptest.Server {
	t.Helper()
	return newServerWithTokens(t, dailyLimit, NewTokenStore(map[string]string{"alice": testToken}))
}

func newServerWithTokens(t *testing.T, dailyLimit int, tokens *TokenStore) *httptest.Server {
	t.Helper()
	wx := weixinStub(t)

	server, err := New(Options{
		Client:         &weixin.Client{BaseURL: wx.URL, HTTPClient: wx.Client()},
		AccessToken:    func(context.Context) (string, error) { return "access-token", nil },
		Tokens:         tokens,
		DailyCallLimit: dailyLimit,
		Logger:         log.New(io.Discard, "", 0),
		Version:        "test",
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// authTransport adds the bearer token every request needs.
type authTransport struct{ token string }

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+a.token)
	return http.DefaultTransport.RoundTrip(clone)
}

func connect(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: authTransport{token: token}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool call failed: %s", textOf(res))
	}
	return textOf(res)
}

func textOf(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestServerRefusesToStartWithoutTokens(t *testing.T) {
	_, err := New(Options{
		Client:      &weixin.Client{},
		AccessToken: func(context.Context) (string, error) { return "", nil },
		Tokens:      NewTokenStore(nil),
	})
	if err == nil {
		t.Fatal("New() error = nil, want error when no bearer token is configured")
	}
}

func TestToolsAreExactlyTheThreeReadOnlyQueries(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil {
			t.Fatalf("tool %s has no annotations", tool.Name)
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %s is not marked read-only", tool.Name)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("tool %s advertises destructive behaviour, which forces approval on every call", tool.Name)
		}
	}
	sort.Strings(names)
	want := "get_drama,list_dramas,published_dramas"
	if strings.Join(names, ",") != want {
		t.Fatalf("tools = %v, want %s", names, want)
	}

	// The delete capability must not exist on the agent side at all.
	if strings.Contains(strings.Join(names, ","), "delete") {
		t.Fatal("a destructive tool is exposed over MCP")
	}
}

func TestListDramasSchemaAdvertisesStateEnum(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "list_dramas" {
			continue
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal input schema: %v", err)
		}
		schema := string(data)
		for _, state := range dramaquery.States {
			if !strings.Contains(schema, string(state)) {
				t.Errorf("input schema does not advertise state %q: %s", state, schema)
			}
		}
		if strings.Contains(schema, "delete") {
			t.Error("input schema mentions delete")
		}
		// The enum makes an invalid state impossible; this description is what
		// lets a model recover when its own value is rejected.
		if !strings.Contains(schema, "可选值") {
			t.Error("state description does not list the accepted values")
		}
		return
	}
	t.Fatal("list_dramas tool not found")
}

func TestPublishedDramasAnswersTheHeadlineQuestion(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "published_dramas"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var env dramaquery.PublishedEnvelope
	if err := json.Unmarshal([]byte(toolText(t, res)), &env); err != nil {
		t.Fatalf("decode published envelope: %v", err)
	}
	if env.Matched != 3 || env.Returned != 3 || env.Truncated {
		t.Errorf("envelope = %+v, want 3 matched / 3 returned / not truncated", env)
	}
}

func TestListDramasFiltersByState(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	cases := []struct {
		name  string
		args  any
		want  int
		state string
	}{
		{"array form", map[string]any{"state": []string{"approved"}}, 2, dramaquery.AuditApproved},
		{"scalar form", map[string]any{"state": "taken-down"}, 1, ""},
		{"union", map[string]any{"state": []string{"taken-down", "returned"}}, 2, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "list_dramas",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			var env dramaquery.Envelope
			if err := json.Unmarshal([]byte(toolText(t, res)), &env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if env.Matched != tc.want {
				t.Errorf("matched = %d, want %d", env.Matched, tc.want)
			}
			// Counts always describe the whole account, not the filter.
			if env.Counts.AuditStatus[dramaquery.AuditInReview] != 1 || env.Counts.TakenDown != 1 {
				t.Errorf("counts = %+v, want whole-list counts", env.Counts)
			}
			if tc.state != "" {
				for _, item := range env.Dramas {
					if item[dramaquery.FieldAuditStatus] != tc.state {
						t.Errorf("record %+v is not %s", item, tc.state)
					}
				}
			}
		})
	}
}

func TestListDramasCapsOutputAndSaysSo(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_dramas",
		Arguments: map[string]any{"max_items": 1},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	var env dramaquery.Envelope
	if err := json.Unmarshal([]byte(toolText(t, res)), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Matched != 4 || env.Returned != 1 || !env.Truncated {
		t.Errorf("envelope = %+v, want 4 matched / 1 returned / truncated", env)
	}
}

func TestToolErrorsAreReportedAsToolResults(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	t.Run("unknown state", func(t *testing.T) {
		// The enum lives in the input schema, so the protocol layer rejects an
		// invalid state before the handler ever sees it. The valid values are
		// part of the schema description, which is what lets the model retry.
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "list_dramas",
			Arguments: map[string]any{"state": "审核中"},
		})
		if err != nil {
			t.Fatalf("CallTool returned a protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
		if !strings.Contains(textOf(res), "state") {
			t.Errorf("error text does not point at the offending argument: %s", textOf(res))
		}
	})

	t.Run("unknown field", func(t *testing.T) {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "list_dramas",
			Arguments: map[string]any{"fields": "dramaId"},
		})
		if err != nil {
			t.Fatalf("CallTool returned a protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
	})

	t.Run("get_drama surfaces the WeChat error", func(t *testing.T) {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "get_drama",
			Arguments: map[string]any{"drama_id": 999},
		})
		if err != nil {
			t.Fatalf("CallTool returned a protocol error: %v", err)
		}
		if !res.IsError {
			t.Fatal("IsError = false, want true")
		}
	})
}

func TestGetDramaAddsDerivedKeys(t *testing.T) {
	srv := newServer(t, 0)
	session := connect(t, srv.URL, testToken)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_drama",
		Arguments: map[string]any{"drama_id": 2},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal([]byte(toolText(t, res)), &record); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	if record[dramaquery.FieldAuditStatus] != dramaquery.AuditApproved {
		t.Errorf("audit_status = %v, want approved", record[dramaquery.FieldAuditStatus])
	}
	if record[dramaquery.FieldTakenDown] != true {
		t.Errorf("taken_down = %v, want true", record[dramaquery.FieldTakenDown])
	}
	// Every raw field is still there, so the answer can be checked by hand.
	if record["name"] != "通过但下架" {
		t.Errorf("raw name lost: %v", record["name"])
	}
}

func TestUnauthorizedRequestsAreRejected(t *testing.T) {
	srv := newServer(t, 0)

	cases := []struct {
		name   string
		header string
	}{
		{"no header", ""},
		{"wrong token", "Bearer nope"},
		{"not bearer", "Basic " + testToken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("{}"))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}

func TestDailyCallLimitStopsARunawayAgent(t *testing.T) {
	srv := newServer(t, 1)
	session := connect(t, srv.URL, testToken)

	first, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "published_dramas"})
	if err != nil {
		t.Fatalf("first CallTool: %v", err)
	}
	if first.IsError {
		t.Fatalf("first call failed: %s", textOf(first))
	}

	second, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "published_dramas"})
	if err != nil {
		t.Fatalf("second CallTool returned a protocol error: %v", err)
	}
	if !second.IsError || !strings.Contains(textOf(second), "上限") {
		t.Fatalf("second call = IsError %v, text %q; want a daily-limit tool error",
			second.IsError, textOf(second))
	}
}

func TestLoadTokenStore(t *testing.T) {
	t.Run("loads names", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "tokens.yaml")
		if err := os.WriteFile(path, []byte("tokens:\n  - name: alice\n    token: tok-a\n"), 0o600); err != nil {
			t.Fatalf("write tokens: %v", err)
		}
		store, err := LoadTokenStore(path)
		if err != nil {
			t.Fatalf("LoadTokenStore() error = %v", err)
		}
		name, ok := store.lookup("tok-a")
		if !ok || name != "alice" {
			t.Fatalf("lookup = %q, %v; want alice, true", name, ok)
		}
		if _, ok := store.lookup("tok-b"); ok {
			t.Error("lookup of an unknown token succeeded")
		}
	})

	t.Run("rejects an empty or broken file", func(t *testing.T) {
		dir := t.TempDir()
		for name, content := range map[string]string{
			"empty.yaml":     "tokens: []\n",
			"noname.yaml":    "tokens:\n  - token: tok-a\n",
			"notoken.yaml":   "tokens:\n  - name: alice\n",
			"duplicate.yaml": "tokens:\n  - name: alice\n    token: tok-a\n  - name: bob\n    token: tok-a\n",
		} {
			path := filepath.Join(dir, name)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
			if _, err := LoadTokenStore(path); err == nil {
				t.Errorf("LoadTokenStore(%s) error = nil, want error", name)
			}
		}
		if _, err := LoadTokenStore(filepath.Join(dir, "missing.yaml")); err == nil {
			t.Error("LoadTokenStore(missing) error = nil, want error")
		}
	})
}
