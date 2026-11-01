package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/mcp"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// baseMs is the received time of fb_0001; later rows are a day apart.
var baseMs = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC).UnixMilli()

const day = 24 * time.Hour

type env struct {
	t        *testing.T
	db       *store.DB
	svc      *service.Service
	srv      *httptest.Server
	log      *syncBuf
	tenant   store.Tenant
	channel  store.Channel
	admin    store.User
	reviewer store.User
	adminTok string
	revTok   string
	adminTID string // id of the admin's token row
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	buf := &syncBuf{}
	log := slog.New(slog.NewTextHandler(buf, nil))
	e := &env{t: t, db: db, log: buf}
	var err error
	if e.tenant, err = store.CreateTenant(ctx, db.SQL, db.Dialect, "Acme", 1, 1000); err != nil {
		t.Fatal(err)
	}
	if e.channel, err = store.CreateChannel(ctx, db.SQL, db.Dialect, e.tenant.ID, "web", 1000); err != nil {
		t.Fatal(err)
	}
	e.admin = e.newUser("admin@acme.test", "admin")
	e.reviewer = e.newUser("rev@acme.test", "reviewer")
	e.svc = &service.Service{DB: db, Log: log, Version: "test", Mode: "all"}
	var tok store.Token
	tok, e.adminTok = e.token(e.admin)
	e.adminTID = tok.ID
	_, e.revTok = e.token(e.reviewer)

	h := mcp.New(e.svc, log)
	mux := http.NewServeMux()
	mux.Handle("/mcp", h)
	mux.Handle("/mcp/", h)
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	e.seed()
	return e
}

func (e *env) newUser(email, role string) store.User {
	e.t.Helper()
	u, err := store.CreateUser(context.Background(), e.db.SQL, e.db.Dialect, store.User{
		TenantID: e.tenant.ID, Email: email, PasswordHash: "x", Role: role, CreatedAt: 1000, UpdatedAt: 1000,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func principalOf(u store.User) ext.Principal {
	return ext.Principal{UserID: u.ID, TenantID: u.TenantID, Role: ext.Role(u.Role), Via: "ui"}
}

func (e *env) token(u store.User) (store.Token, string) {
	e.t.Helper()
	row, plain, err := e.svc.CreateToken(context.Background(), principalOf(u), "test")
	if err != nil {
		e.t.Fatal(err)
	}
	return row, plain
}

// seed inserts fb_0001 to fb_0005, a day apart; fb_0003 has metadata.
func (e *env) seed() {
	e.t.Helper()
	var rows []store.FeedbackRow
	for i := 1; i <= 5; i++ {
		r := store.FeedbackRow{
			ID: fmt.Sprintf("fb_%04d", i), TenantID: e.tenant.ID, ChannelID: e.channel.ID,
			Message: fmt.Sprintf("message %d\nsecond line", i), IdempotencyKey: fmt.Sprintf("k%d", i),
			ReceivedAt: baseMs + int64(i-1)*day.Milliseconds(),
		}
		if i == 3 {
			r.Metadata = []byte(`{"plan":"pro"}`)
		}
		rows = append(rows, r)
	}
	if _, err := store.InsertFeedbackBatch(context.Background(), e.db.SQL, e.db.Dialect, rows); err != nil {
		e.t.Fatal(err)
	}
}

// bearer sets the Authorization header on a clone of each request.
type bearer struct {
	tok  string
	base http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.tok)
	base := b.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}

func (e *env) connectWith(tok string, rt http.RoundTripper) *sdk.ClientSession {
	e.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.StreamableClientTransport{
		Endpoint:             e.srv.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearer{tok: tok, base: rt}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		e.t.Fatalf("connect: %v", err)
	}
	e.t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func (e *env) connect(tok string) *sdk.ClientSession { return e.connectWith(tok, nil) }

// call runs a tool and fails the test on a protocol error.
func call(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) *sdk.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// ok runs a tool, requires success and decodes the structured output into out.
func ok(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res := call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s: tool error: %s", name, text(res))
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: decode %s: %v", name, b, err)
	}
}

// fails runs a tool, requires a tool error and returns its text.
func fails(t *testing.T, cs *sdk.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s: want a tool error, got %v", name, res.StructuredContent)
	}
	return text(res)
}

func text(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`

// raw sends a request with the given headers and returns the response and body.
func raw(t *testing.T, method, url string, hdr map[string]string, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// postInit sends a valid initialize request with the given token ("" = none).
func (e *env) postInit(t *testing.T, tok string) (*http.Response, string) {
	hdr := map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
	if tok != "" {
		hdr["Authorization"] = "Bearer " + tok
	}
	return raw(t, http.MethodPost, e.srv.URL+"/mcp", hdr, initBody)
}

type feedbackItem struct {
	ID          string   `json:"id"`
	ChannelID   string   `json:"channelId"`
	ChannelName *string  `json:"channelName"`
	Message     string   `json:"message"`
	Metadata    any      `json:"metadata"`
	Status      string   `json:"status"`
	ReceivedAt  string   `json:"receivedAt"`
	UpdatedBy   *string  `json:"updatedBy"`
	Tags        []string `json:"tags"`
}

type listOut struct {
	Items      []feedbackItem `json:"items"`
	NextCursor *string        `json:"nextCursor"`
}

type bulkRes struct {
	Updated  int      `json:"updated"`
	NotFound []string `json:"notFound"`
}

func ids(items []feedbackItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.ID
	}
	return out
}
