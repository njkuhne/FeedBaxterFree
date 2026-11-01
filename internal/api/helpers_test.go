package api_test

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

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/api"
	"github.com/njkuhne/feedbaxter/internal/ingest/keysync"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxter/internal/selfhost"
	"github.com/njkuhne/feedbaxter/internal/service"
	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

const testPassword = "correct horse battery"

var (
	hashOnce sync.Once
	hashVal  string
)

func testHash(t testing.TB) string {
	hashOnce.Do(func() {
		var err error
		if hashVal, err = service.HashPassword(testPassword); err != nil {
			t.Fatal(err)
		}
	})
	return hashVal
}

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

type noLimits struct{}

func (noLimits) Limits(context.Context, string) (ext.Limits, error) { return ext.Limits{}, nil }

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
}

func newEnv(t *testing.T, db *store.DB) *env {
	t.Helper()
	ctx := context.Background()
	_, obj := queuetest.Embedded(t, queue.Limits{})
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
	e.admin = e.newUser("admin@acme.test", "admin", false)
	e.reviewer = e.newUser("rev@acme.test", "reviewer", false)
	e.svc = &service.Service{
		DB: db, Queue: obj, Log: log, Version: "test", Mode: "all",
		Keys: &keysync.Writer{DB: db, KV: obj.Keys, Limits: noLimits{}, Log: log},
	}
	sessions := api.NewSessionStore(e.svc)
	auth := &selfhost.Auth{Svc: e.svc, Sessions: sessions, Tenants: selfhost.TenantResolver{DB: db}}
	h := api.New(api.Config{Svc: e.svc, Auth: auth, Deps: ext.Deps{Logger: log, Sessions: sessions}, Log: log})
	mux := http.NewServeMux()
	mux.Handle("/api/", h)
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	e.adminTok, e.revTok = e.session(e.admin), e.session(e.reviewer)
	return e
}

func (e *env) newUser(email, role string, must bool) store.User {
	e.t.Helper()
	u, err := store.CreateUser(context.Background(), e.db.SQL, e.db.Dialect, store.User{
		TenantID: e.tenant.ID, Email: email, PasswordHash: testHash(e.t), Role: role,
		MustChangePassword: must, CreatedAt: 1000, UpdatedAt: 1000,
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

// session starts a session without going through login (which costs a hash).
func (e *env) session(u store.User) string {
	e.t.Helper()
	tok, _, err := e.svc.CreateSession(context.Background(), u)
	if err != nil {
		e.t.Fatal(err)
	}
	return tok
}

func (e *env) principal(u store.User) ext.Principal {
	return ext.Principal{UserID: u.ID, TenantID: u.TenantID, Role: ext.Role(u.Role), Via: "ui"}
}

type reply struct {
	status int
	header http.Header
	body   []byte
	cookie []*http.Cookie
}

func (r reply) obj(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(r.body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v\n%s", err, r.body)
	}
	return m
}

// req sends a request with the CSRF header unless a modifier removes it.
func (e *env) req(method, path string, body any, mods ...func(*http.Request)) reply {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			e.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	r, err := http.NewRequest(method, e.srv.URL+path, rd)
	if err != nil {
		e.t.Fatal(err)
	}
	r.Header.Set("X-Requested-With", "test")
	for _, m := range mods {
		m(r)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatal(err)
	}
	return reply{status: resp.StatusCode, header: resp.Header, body: raw, cookie: resp.Cookies()}
}

func cookie(tok string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: api.SessionCookieName, Value: tok}) }
}

// header sets a request header; an empty value removes it.
func header(k, v string) func(*http.Request) {
	return func(r *http.Request) {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
}

func (e *env) asAdmin(method, path string, body any, mods ...func(*http.Request)) reply {
	e.t.Helper()
	return e.req(method, path, body, append([]func(*http.Request){cookie(e.adminTok)}, mods...)...)
}

func (e *env) asReviewer(method, path string, body any) reply {
	e.t.Helper()
	return e.req(method, path, body, cookie(e.revTok))
}

func (e *env) want(r reply, status int) reply {
	e.t.Helper()
	if r.status != status {
		e.t.Fatalf("status = %d, want %d; body %s", r.status, status, r.body)
	}
	return r
}

func (e *env) wantErr(r reply, status int, code string) {
	e.t.Helper()
	e.want(r, status)
	m := r.obj(e.t)
	msg, _ := m["message"].(string)
	if m["error"] != code || msg == "" || len(m) != 2 {
		e.t.Fatalf("error body = %s, want code %q and a message only", r.body, code)
	}
}

func (e *env) sessionCookie(r reply) *http.Cookie {
	e.t.Helper()
	for _, c := range r.cookie {
		if c.Name == api.SessionCookieName {
			return c
		}
	}
	e.t.Fatalf("no %s cookie in response", api.SessionCookieName)
	return nil
}

// seed inserts feedback rows directly; ids are fb_001, fb_002, ... and fb_N is N-1 days after 2026-03-01 12:00 UTC.
func (e *env) seed(msgs ...string) []string {
	e.t.Helper()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	rows := make([]store.FeedbackRow, len(msgs))
	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ids[i] = fmt.Sprintf("fb_%04d", i+1)
		rows[i] = store.FeedbackRow{
			ID: ids[i], TenantID: e.tenant.ID, ChannelID: e.channel.ID, Message: m,
			IdempotencyKey: ids[i], ReceivedAt: base.AddDate(0, 0, i).UnixMilli(),
		}
	}
	if _, err := store.InsertFeedbackBatch(context.Background(), e.db.SQL, e.db.Dialect, rows); err != nil {
		e.t.Fatal(err)
	}
	return ids
}

func itemsOf(t *testing.T, r reply) []map[string]any {
	t.Helper()
	raw, ok := r.obj(t)["items"].([]any)
	if !ok {
		t.Fatalf("no items array in %s", r.body)
	}
	out := make([]map[string]any, len(raw))
	for i, x := range raw {
		out[i] = x.(map[string]any)
	}
	return out
}

func idsOf(items []map[string]any) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it["id"].(string)
	}
	return out
}

func eachDialect(t *testing.T, fn func(t *testing.T, e *env)) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) { fn(t, newEnv(t, db)) })
}

type fixedAuth struct{ p ext.Principal }

func (fixedAuth) Mount(*http.ServeMux, ext.Deps)                    {}
func (f fixedAuth) Principal(*http.Request) (ext.Principal, bool) { return f.p, true }
