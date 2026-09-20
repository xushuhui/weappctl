// Package mcpserver exposes weappctl's read-only drama queries over the Model
// Context Protocol, so colleagues who do not use a shell can ask questions in
// their agent instead of running the CLI.
//
// Two things this package deliberately does NOT do: it never exposes
// deleteMedia (the agent side must not have that capability at all, see
// docs/adr/0002), and it never returns an unbounded list (see
// dramaquery.DefaultFields and DefaultMaxItems).
package mcpserver

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/xsh/weappctl/internal/dramaquery"
	"github.com/xsh/weappctl/internal/weixin"
)

// DefaultMaxItems caps how many records a list tool returns when the caller
// does not ask for a specific cap. A bare list must never dump the whole
// account (about 3 MB for 1150 dramas) into an agent's context.
const DefaultMaxItems = 200

// DefaultDailyCallLimit caps tool calls per person per day, so a looping agent
// cannot burn the account's WeChat API quota.
const DefaultDailyCallLimit = 2000

// Options wires a Server to the account it serves.
type Options struct {
	// Client calls the WeChat API; its BaseURL is overridable in tests.
	Client *weixin.Client
	// AccessToken returns a valid access token for the served account.
	AccessToken func(context.Context) (string, error)
	// Tokens holds the per-person bearer tokens.
	Tokens *TokenStore
	// DailyCallLimit caps tool calls per person per day; zero disables the cap.
	DailyCallLimit int
	// Logger receives one line per tool call. Defaults to stderr.
	Logger *log.Logger
	// Version is reported in the MCP handshake.
	Version string
}

// Server serves the read-only drama tools over streamable HTTP.
type Server struct {
	client      *weixin.Client
	accessToken func(context.Context) (string, error)
	tokens      *TokenStore
	dailyLimit  int
	logger      *log.Logger
	version     string

	mu    sync.Mutex
	usage map[string]*usage
}

type usage struct {
	day   string
	count int
}

// New validates opts and returns a ready Server.
func New(opts Options) (*Server, error) {
	if opts.Client == nil {
		return nil, errors.New("mcpserver: Client is required")
	}
	if opts.AccessToken == nil {
		return nil, errors.New("mcpserver: AccessToken is required")
	}
	if opts.Tokens == nil || opts.Tokens.empty() {
		return nil, errors.New("mcpserver: at least one bearer token is required")
	}
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	return &Server{
		client:      opts.Client,
		accessToken: opts.AccessToken,
		tokens:      opts.Tokens,
		dailyLimit:  opts.DailyCallLimit,
		logger:      logger,
		version:     opts.Version,
		usage:       map[string]*usage{},
	}, nil
}

// Handler returns the authenticated HTTP handler for the MCP endpoint.
func (s *Server) Handler() http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(s.serverForRequest, nil)
	return s.authenticate(mcpHandler)
}

type identityKey struct{}

// authenticate requires a per-person bearer token on every request, including
// session handshakes and follow-up calls, and attaches the caller's name for
// logging and the daily cap.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			s.reject(w, r, "missing bearer token")
			return
		}
		name, ok := s.tokens.lookup(token)
		if !ok {
			s.reject(w, r, "unknown bearer token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, name)))
	})
}

func (s *Server) reject(w http.ResponseWriter, r *http.Request, reason string) {
	s.logger.Printf("rejected %s %s from %s: %s", r.Method, r.URL.Path, r.RemoteAddr, reason)
	w.Header().Set("WWW-Authenticate", `Bearer realm="weappctl"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// serverForRequest builds the MCP server for a new session. The SDK calls this
// once per session, so the caller's name is captured here and used to label
// tool calls.
//
// ASSUMPTION: a session is only used by the client that opened it. Nothing
// security-relevant depends on this — every HTTP request is authenticated
// independently — but if that changed, log lines could be misattributed.
func (s *Server) serverForRequest(r *http.Request) *mcp.Server {
	name, _ := r.Context().Value(identityKey{}).(string)
	return s.newServer(name)
}

func (s *Server) newServer(caller string) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:        "weappctl",
		Title:       "微信小程序短剧数据（只读）",
		Version:     s.version,
		Description: "查询该小程序提交过的短剧：已上架清单、审核状态分布、单部剧详情。只读。",
	}, nil)

	mcp.AddTool(srv, listDramasTool(), s.listDramas(caller))
	mcp.AddTool(srv, publishedDramasTool(), s.publishedDramas(caller))
	mcp.AddTool(srv, getDramaTool(), s.getDrama(caller))

	return srv
}

// readOnly marks a tool as not modifying anything. DestructiveHint is set
// explicitly to false: per the MCP spec it defaults to true, and hosts such as
// Codex always require approval for a tool that advertises destructiveness —
// even when it also advertises read-only.
func readOnly() *mcp.ToolAnnotations {
	no := false
	return &mcp.ToolAnnotations{
		Title:           "只读查询",
		ReadOnlyHint:    true,
		DestructiveHint: &no,
		IdempotentHint:  true,
	}
}

func listDramasTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "list_dramas",
		Description: "查询该小程序提交过的全部短剧（含未上架、审核中、被退回的），可按审核状态过滤。" +
			"返回：matched（符合条件的总数）、counts（全量统计，不受过滤影响）、dramas（默认最多 " +
			"200 条精简记录，truncated 表示是否被截断）。" +
			"审核阶段看 audit_status：in-review 审核中、returned 退回待修改、rejected 终审拒绝、" +
			"approved 审核通过（只代表有资格上架）。taken_down 是平台下架标记，与审核状态是两个独立" +
			"维度，一部剧可以同时是 approved 且 taken_down。" +
			"注意：问“有多少短剧上架/在架”必须用 published_dramas，不能用 approved——审核通过不等于已上架。",
		InputSchema: listDramasSchema(),
		Annotations: readOnly(),
	}
}

func listDramasSchema() json.RawMessage {
	stateSchema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "string",
			"enum": stateWords(),
		},
		"description": "按状态过滤，可多选（满足任意一个即算匹配）。可选值：" + strings.Join(stateWords(), ", ") + "。",
	}
	return mustSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"state": map[string]any{
				"description": stateSchema["description"],
				"oneOf": []any{
					map[string]any{"type": "string", "enum": stateWords()},
					stateSchema,
				},
			},
			"fields": map[string]any{
				"type": "string",
				"description": "逗号分隔的输出字段；默认 " + strings.Join(dramaquery.DefaultFields, ",") +
					"。可选字段：" + strings.Join(dramaquery.FieldNames(), ", "),
			},
			"max_items": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": fmt.Sprintf("最多返回多少条；默认 %d，0 表示不限。", DefaultMaxItems),
			},
		},
		"additionalProperties": false,
	})
}

func publishedDramasTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "published_dramas",
		Description: "查询该小程序当前“已上架”的短剧清单和数量（微信 developerGetPublishedDrama）。" +
			"“上架”指显式执行过短剧上架动作，与“审核通过”是两回事——审核通过只代表有资格上架。" +
			"回答“有多少短剧上架/在架”用这个工具。" +
			"返回：matched（已上架总数，直接作为答案）、dramas（默认最多 200 条，每条含 drama_id、" +
			"src_appid（提审方小程序）、publish_time）、truncated。",
		InputSchema: maxItemsSchema(),
		Annotations: readOnly(),
	}
}

func getDramaTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "get_drama",
		Description: "按 drama_id 查询单部剧的完整原始信息（含剧名、制作方、集数、媒资列表、" +
			"审核详情等），并附带两个解释性字段：audit_status（权威审核阶段）与 taken_down（是否被平台下架）。" +
			"原始 status 字段是粗粒度的可播标记，判断审核阶段请以 audit_status 为准。",
		InputSchema: mustSchema(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"drama_id": map[string]any{
					"type":        "integer",
					"description": "剧目 ID（数字型，来自 list_dramas）。",
				},
			},
			"required":             []string{"drama_id"},
			"additionalProperties": false,
		}),
		Annotations: readOnly(),
	}
}

func maxItemsSchema() json.RawMessage {
	return mustSchema(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"max_items": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": fmt.Sprintf("最多返回多少条；默认 %d，0 表示不限。", DefaultMaxItems),
			},
		},
		"additionalProperties": false,
	})
}

func mustSchema(schema map[string]any) json.RawMessage {
	data, err := json.Marshal(schema)
	if err != nil {
		// The schema is a literal built here; a marshal failure is a bug.
		panic(fmt.Sprintf("mcpserver: build tool schema: %v", err))
	}
	return data
}

func stateWords() []string {
	words := make([]string, 0, len(dramaquery.States))
	for _, s := range dramaquery.States {
		words = append(words, string(s))
	}
	return words
}

// stringList accepts either a single string or an array of strings, because
// models routinely send a scalar for an array-shaped parameter.
type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*l = stringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("expected a string or an array of strings: %w", err)
	}
	*l = many
	return nil
}

type listDramasArgs struct {
	State    stringList `json:"state,omitempty"`
	Fields   string     `json:"fields,omitempty"`
	MaxItems *int       `json:"max_items,omitempty"`
}

type publishedDramasArgs struct {
	MaxItems *int `json:"max_items,omitempty"`
}

type getDramaArgs struct {
	DramaID int64 `json:"drama_id"`
}

func (s *Server) listDramas(caller string) mcp.ToolHandlerFor[listDramasArgs, dramaquery.Envelope] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args listDramasArgs) (*mcp.CallToolResult, dramaquery.Envelope, error) {
		started := time.Now()
		var empty dramaquery.Envelope

		if err := s.allowCall(caller); err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}

		states, err := dramaquery.ParseStates(args.State)
		if err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}
		fields, err := dramaquery.ParseFields(args.Fields)
		if err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}
		maxItems, err := resolveMaxItems(args.MaxItems)
		if err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}

		dramas, err := s.fetchAllDramas(ctx)
		if err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}

		env, err := dramaquery.Build(dramas, states, fields, maxItems)
		if err != nil {
			s.logCall(caller, "list_dramas", args.describe(), 0, started, err)
			return nil, empty, err
		}

		s.logCall(caller, "list_dramas", args.describe(), env.Matched, started, nil)
		return nil, env, nil
	}
}

func (s *Server) publishedDramas(caller string) mcp.ToolHandlerFor[publishedDramasArgs, dramaquery.PublishedEnvelope] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args publishedDramasArgs) (*mcp.CallToolResult, dramaquery.PublishedEnvelope, error) {
		started := time.Now()
		var empty dramaquery.PublishedEnvelope

		if err := s.allowCall(caller); err != nil {
			s.logCall(caller, "published_dramas", "", 0, started, err)
			return nil, empty, err
		}
		maxItems, err := resolveMaxItems(args.MaxItems)
		if err != nil {
			s.logCall(caller, "published_dramas", "", 0, started, err)
			return nil, empty, err
		}

		token, err := s.accessToken(ctx)
		if err != nil {
			s.logCall(caller, "published_dramas", "", 0, started, err)
			return nil, empty, err
		}
		pubs, err := s.client.GetPublishedDramas(ctx, token)
		if err != nil {
			s.logCall(caller, "published_dramas", "", 0, started, err)
			return nil, empty, fmt.Errorf("获取已上架短剧失败: %w", err)
		}

		env, err := dramaquery.BuildPublished(pubs, maxItems)
		if err != nil {
			s.logCall(caller, "published_dramas", "", 0, started, err)
			return nil, empty, err
		}

		s.logCall(caller, "published_dramas", "", env.Matched, started, nil)
		return nil, env, nil
	}
}

func (s *Server) getDrama(caller string) mcp.ToolHandlerFor[getDramaArgs, map[string]any] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args getDramaArgs) (*mcp.CallToolResult, map[string]any, error) {
		started := time.Now()
		var empty map[string]any
		detail := fmt.Sprintf("drama_id=%d", args.DramaID)

		if err := s.allowCall(caller); err != nil {
			s.logCall(caller, "get_drama", detail, 0, started, err)
			return nil, empty, err
		}
		if args.DramaID <= 0 {
			err := fmt.Errorf("drama_id 必须是正整数，当前为 %d", args.DramaID)
			s.logCall(caller, "get_drama", detail, 0, started, err)
			return nil, empty, err
		}

		token, err := s.accessToken(ctx)
		if err != nil {
			s.logCall(caller, "get_drama", detail, 0, started, err)
			return nil, empty, err
		}
		info, err := s.client.GetDrama(ctx, token, args.DramaID)
		if err != nil {
			s.logCall(caller, "get_drama", detail, 0, started, err)
			return nil, empty, fmt.Errorf("获取剧目信息失败: %w", err)
		}

		record, err := dramaquery.ProjectWithDerived(*info)
		if err != nil {
			s.logCall(caller, "get_drama", detail, 0, started, err)
			return nil, empty, err
		}

		s.logCall(caller, "get_drama", detail, 1, started, nil)
		return nil, record, nil
	}
}

func (s *Server) fetchAllDramas(ctx context.Context) ([]weixin.DramaInfo, error) {
	token, err := s.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	dramas, err := dramaquery.FetchAllDramas(ctx, s.client, token)
	if err != nil {
		return nil, fmt.Errorf("获取剧目列表失败: %w", err)
	}
	return dramas, nil
}

func resolveMaxItems(value *int) (int, error) {
	if value == nil {
		return DefaultMaxItems, nil
	}
	if err := dramaquery.ValidateMaxItems(*value); err != nil {
		return 0, err
	}
	return *value, nil
}

func (a listDramasArgs) describe() string {
	parts := make([]string, 0, 3)
	if len(a.State) > 0 {
		parts = append(parts, "state="+strings.Join(a.State, "|"))
	}
	if a.Fields != "" {
		parts = append(parts, "fields="+a.Fields)
	}
	if a.MaxItems != nil {
		parts = append(parts, fmt.Sprintf("max_items=%d", *a.MaxItems))
	}
	return strings.Join(parts, " ")
}

// allowCall enforces the per-person daily cap set by
// DefaultDailyCallLimit / --daily-call-limit.
func (s *Server) allowCall(caller string) error {
	if s.dailyLimit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	u := s.usage[caller]
	if u == nil || u.day != today {
		u = &usage{day: today}
		s.usage[caller] = u
	}
	if u.count >= s.dailyLimit {
		return fmt.Errorf("已达到今日调用上限（%d 次），请明天再试", s.dailyLimit)
	}
	u.count++
	return nil
}

func (s *Server) logCall(caller, tool, detail string, matched int, started time.Time, err error) {
	status := "ok"
	if err != nil {
		status = "error: " + err.Error()
	}
	s.logger.Printf("caller=%s tool=%s %smatched=%d took=%s status=%s",
		caller, tool, detail+" ", matched, time.Since(started).Round(time.Millisecond), status)
}

func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

// TokenStore maps per-person bearer tokens to names, so that usage logs
// identify a person and a single token can be revoked without rotating
// everyone else's.
type TokenStore struct {
	byToken map[string]string
}

type tokenFile struct {
	Tokens []struct {
		Name  string `yaml:"name"`
		Token string `yaml:"token"`
	} `yaml:"tokens"`
}

// LoadTokenStore reads the tokens file. A missing file, an empty list, or an
// entry without a name or token is an error: the server must never start
// without anyone able to authenticate, and an unnamed token would make the
// usage log useless.
func LoadTokenStore(path string) (*TokenStore, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tokens file %s: %w", path, err)
	}
	var file tokenFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tokens file %s: %w", path, err)
	}

	store := &TokenStore{byToken: map[string]string{}}
	for i, entry := range file.Tokens {
		if entry.Name == "" {
			return nil, fmt.Errorf("tokens file %s: entry %d is missing name", path, i+1)
		}
		if entry.Token == "" {
			return nil, fmt.Errorf("tokens file %s: entry %q is missing token", path, entry.Name)
		}
		if _, dup := store.byToken[entry.Token]; dup {
			return nil, fmt.Errorf("tokens file %s: duplicate token for %q", path, entry.Name)
		}
		store.byToken[entry.Token] = entry.Name
	}
	if len(store.byToken) == 0 {
		return nil, fmt.Errorf("tokens file %s contains no tokens", path)
	}
	return store, nil
}

// NewTokenStore builds a store from name to token pairs; used by tests.
func NewTokenStore(entries map[string]string) *TokenStore {
	store := &TokenStore{byToken: make(map[string]string, len(entries))}
	for name, token := range entries {
		store.byToken[token] = name
	}
	return store
}

func (s *TokenStore) empty() bool { return s == nil || len(s.byToken) == 0 }

// lookup resolves a bearer token to its owner's name, comparing every entry in
// constant time so that response timing does not reveal how much of a guessed
// token was correct.
func (s *TokenStore) lookup(token string) (string, bool) {
	var name string
	found := 0
	for candidate, owner := range s.byToken {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			name = owner
			found = 1
		}
	}
	return name, found == 1
}
