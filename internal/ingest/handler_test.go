package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
)

const (
	testKey    = "fbx_testkey"
	testTenant = "tn_1"
	testChan   = "ch_1"
	testKeyID  = "key_1"
)

type env struct {
	conn  *queue.Conn
	objs  *queue.Objects
	cache *KeyCache
	cn    *Counters
	h     http.Handler
	now   time.Time
}

type envOpts struct {
	entry   Entry
	perKey  int
	qlim    queue.Limits
	now     time.Time
	noCache bool
	ready   <-chan struct{} // Deps.Ready; nil = the cache's
}

func newEnv(t *testing.T, o envOpts) *env {
	t.Helper()
	conn, objs := queuetest.Embedded(t, o.qlim)
	if o.entry.KeyID == "" {
		o.entry.KeyID, o.entry.TenantID, o.entry.ChannelID = testKeyID, testTenant, testChan
	}
	if o.perKey == 0 {
		o.perKey = 1000
	}
	if o.now.IsZero() {
		o.now = time.Date(2026, 3, 10, 12, 0, 0, 123456789, time.UTC)
	}
	e := &env{conn: conn, objs: objs, now: o.now}
	if o.noCache {
		e.cache = NewKeyCache(discardLog()) // never run, so never ready
	} else {
		putEntry(t, objs.Keys, testKey, o.entry)
		e.cache = startCache(t, objs.Keys)
	}
	e.cn = NewCounters(conn.JS, objs.Counters, time.Hour, discardLog())
	e.h = NewHandler(Deps{
		Cache: e.cache, Limiter: NewLimiter(o.perKey, time.Minute), Counters: e.cn, JS: conn.JS,
		Config: Config{MaxBodyBytes: 1024, MaxMessageChars: 20, MaxMetadataBytes: 64, PublishTimeout: 2 * time.Second},
		Ready:  o.ready, Log: discardLog(), Now: func() time.Time { return e.now },
	})
	return e
}

func (e *env) post(body string, mods ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/feedback", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+testKey)
	for _, m := range mods {
		m(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	return w
}

func (e *env) tenantCounts() [2]int64 { return e.cn.Snapshot()[testTenant] }

func (e *env) streamMsgs(t *testing.T) uint64 {
	t.Helper()
	info, err := e.objs.Stream.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return info.State.Msgs
}

func wantError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d, want %d (%s)", w.Code, status, w.Body)
	}
	var b struct{ Error, Message string }
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil || b.Error != code || b.Message == "" {
		t.Fatalf("body %q: %v", w.Body, err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content type %q", ct)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || w.Header().Get("Access-Control-Expose-Headers") != "Retry-After" {
		t.Fatalf("CORS headers missing on error: %v", w.Header())
	}
}

const goodBody = `{"message":"hello"}`

func TestPreflight(t *testing.T) {
	e := newEnv(t, envOpts{})
	r := httptest.NewRequest(http.MethodOptions, "/v1/feedback", nil)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
	want := map[string]string{
		"Access-Control-Allow-Origin":   "*",
		"Access-Control-Allow-Methods":  "POST, OPTIONS",
		"Access-Control-Allow-Headers":  "Authorization, Content-Type",
		"Access-Control-Max-Age":        "86400",
		"Access-Control-Expose-Headers": "Retry-After",
	}
	for k, v := range want {
		if got := w.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	e := newEnv(t, envOpts{})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	wantError(t, w, http.StatusMethodNotAllowed, "method_not_allowed")
}

func TestInvalidKeyUnknown(t *testing.T) {
	e := newEnv(t, envOpts{})
	w := e.post(goodBody, func(r *http.Request) { r.Header.Set("Authorization", "Bearer other") })
	wantError(t, w, http.StatusUnauthorized, "invalid_key")
}

func TestInvalidKeyRevoked(t *testing.T) {
	e := newEnv(t, envOpts{entry: Entry{KeyID: testKeyID, TenantID: testTenant, ChannelID: testChan, Revoked: true}})
	wantError(t, e.post(goodBody), http.StatusUnauthorized, "invalid_key")
}

func TestInvalidKeyMissingHeader(t *testing.T) {
	e := newEnv(t, envOpts{})
	wantError(t, e.post(goodBody, func(r *http.Request) { r.Header.Del("Authorization") }), http.StatusUnauthorized, "invalid_key")
	wantError(t, e.post(goodBody, func(r *http.Request) { r.Header.Set("Authorization", testKey) }), http.StatusUnauthorized, "invalid_key")
	wantError(t, e.post(goodBody, func(r *http.Request) { r.Header.Set("Authorization", "Bearer ") }), http.StatusUnauthorized, "invalid_key")
	// The scheme is case-insensitive.
	if w := e.post(goodBody, func(r *http.Request) { r.Header.Set("Authorization", "bEaReR "+testKey) }); w.Code != http.StatusAccepted {
		t.Fatalf("lowercase scheme: %d", w.Code)
	}
}

func TestNotReady(t *testing.T) {
	e := newEnv(t, envOpts{noCache: true})
	w := e.post(goodBody)
	wantError(t, w, http.StatusServiceUnavailable, "queue_full")
	if w.Header().Get("Retry-After") != "5" {
		t.Fatalf("Retry-After %q", w.Header().Get("Retry-After"))
	}
}

func TestReadyOverride(t *testing.T) {
	ready := make(chan struct{})
	e := newEnv(t, envOpts{ready: ready}) // the cache is ready; the override is not
	wantError(t, e.post(goodBody), http.StatusServiceUnavailable, "queue_full")
	close(ready)
	if w := e.post(goodBody); w.Code != http.StatusAccepted {
		t.Fatalf("after ready: %d (%s)", w.Code, w.Body)
	}
}

func TestPayloadTooLarge(t *testing.T) {
	e := newEnv(t, envOpts{})
	big := `{"message":"` + strings.Repeat("a", 2000) + `"}`
	wantError(t, e.post(big), http.StatusRequestEntityTooLarge, "payload_too_large")
	if e.tenantCounts() != [2]int64{} {
		t.Fatal("counted")
	}
}

func TestInvalidPayload(t *testing.T) {
	e := newEnv(t, envOpts{})
	multi := strings.Repeat("é", 20) // 40 bytes, exactly the rune limit
	cases := []struct {
		name, body string
		ok         bool
	}{
		{"not json", `nope`, false},
		{"empty body", ``, false},
		{"array", `[1]`, false},
		{"null", `null`, false},
		{"message missing", `{}`, false},
		{"message number", `{"message":1}`, false},
		{"message null", `{"message":null}`, false},
		{"message empty", `{"message":""}`, false},
		{"message whitespace", `{"message":" \n\t"}`, false},
		{"message too long", `{"message":"` + strings.Repeat("a", 21) + `"}`, false},
		{"multibyte at limit", `{"message":"` + multi + `"}`, true},
		{"multibyte over limit", `{"message":"` + multi + `é"}`, false},
		{"metadata array", `{"message":"x","metadata":[1]}`, false},
		{"metadata string", `{"message":"x","metadata":"s"}`, false},
		{"metadata too large", `{"message":"x","metadata":{"k":"` + strings.Repeat("v", 70) + `"}}`, false},
		{"metadata ok", `{"message":"x","metadata":{"k":"v"}}`, true},
		{"metadata null", `{"message":"x","metadata":null}`, true},
		{"idempotencyKey 129", `{"message":"x","idempotencyKey":"` + strings.Repeat("k", 129) + `"}`, false},
		{"idempotencyKey 128", `{"message":"x","idempotencyKey":"` + strings.Repeat("k", 128) + `"}`, true},
		{"idempotencyKey empty", `{"message":"x","idempotencyKey":""}`, false},
		{"idempotencyKey number", `{"message":"x","idempotencyKey":5}`, false},
		{"idempotencyKey newline", `{"message":"x","idempotencyKey":"a\r\nb"}`, false},
		{"userId number", `{"message":"x","userId":5}`, false},
		{"appVersion object", `{"message":"x","appVersion":{}}`, false},
		{"platform bool", `{"message":"x","platform":true}`, false},
		{"unknown fields ignored", `{"message":"x","extra":[1,2],"userId":null}`, true},
	}
	for _, c := range cases {
		before := e.tenantCounts()
		w := e.post(c.body)
		if c.ok {
			if w.Code != http.StatusAccepted {
				t.Errorf("%s: status %d %s", c.name, w.Code, w.Body)
			}
			continue
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", c.name, w.Code)
			continue
		}
		wantError(t, w, http.StatusBadRequest, "invalid_payload")
		if got := e.tenantCounts(); got != before {
			t.Errorf("%s: counters changed %v -> %v", c.name, before, got)
		}
	}
}

func TestRateLimited(t *testing.T) {
	e := newEnv(t, envOpts{perKey: 2})
	for i := 0; i < 2; i++ {
		if w := e.post(goodBody); w.Code != http.StatusAccepted {
			t.Fatalf("%d: %d", i, w.Code)
		}
	}
	w := e.post(goodBody)
	wantError(t, w, http.StatusTooManyRequests, "rate_limited")
	if ra := w.Header().Get("Retry-After"); ra != "30" { // 2 per minute
		t.Fatalf("Retry-After %q", ra)
	}
	if e.tenantCounts() != [2]int64{2, 2} {
		t.Fatalf("rate limited request counted: %v", e.tenantCounts())
	}
}

func TestDailyLimit(t *testing.T) {
	e := newEnv(t, envOpts{
		entry: Entry{KeyID: testKeyID, TenantID: testTenant, ChannelID: testChan, DailyLimit: 1},
		now:   time.Date(2026, 3, 10, 23, 59, 30, 0, time.UTC),
	})
	if w := e.post(goodBody); w.Code != http.StatusAccepted {
		t.Fatalf("first: %d", w.Code)
	}
	w := e.post(goodBody)
	wantError(t, w, http.StatusTooManyRequests, "daily_limit")
	if w.Header().Get("Retry-After") != "30" {
		t.Fatalf("Retry-After %q", w.Header().Get("Retry-After"))
	}
}

func TestPlanLimit(t *testing.T) {
	e := newEnv(t, envOpts{entry: Entry{KeyID: testKeyID, TenantID: testTenant, ChannelID: testChan, MonthlyLimit: 1}})
	if w := e.post(goodBody); w.Code != http.StatusAccepted {
		t.Fatalf("first: %d", w.Code)
	}
	w := e.post(goodBody)
	wantError(t, w, http.StatusTooManyRequests, "plan_limit")
	if _, ok := w.Header()["Retry-After"]; ok {
		t.Fatal("plan_limit must not carry Retry-After")
	}
}

func TestQueueFull(t *testing.T) {
	e := newEnv(t, envOpts{qlim: queue.Limits{MaxMsgs: 1}})
	if w := e.post(`{"message":"one"}`); w.Code != http.StatusAccepted {
		t.Fatalf("first: %d", w.Code)
	}
	w := e.post(`{"message":"two"}`)
	wantError(t, w, http.StatusServiceUnavailable, "queue_full")
	if w.Header().Get("Retry-After") != "5" {
		t.Fatalf("Retry-After %q", w.Header().Get("Retry-After"))
	}
	if e.tenantCounts() != [2]int64{1, 1} {
		t.Fatalf("not refunded: %v", e.tenantCounts())
	}
	if e.streamMsgs(t) != 1 {
		t.Fatal("stream grew")
	}
}

func fetchOne(t *testing.T, e *env) jetstream.Msg {
	t.Helper()
	batch, err := e.objs.Consumer.Fetch(1, jetstream.FetchMaxWait(waitTimeout))
	if err != nil {
		t.Fatal(err)
	}
	for m := range batch.Messages() {
		return m
	}
	t.Fatalf("no message: %v", batch.Error())
	return nil
}

func TestAcceptedPublishesExactMessage(t *testing.T) {
	e := newEnv(t, envOpts{})
	body := `{ "message" : "hello", "idempotencyKey":"idem-1", "metadata":{"a":1}, "extra":true }`
	w := e.post(body)
	if w.Code != http.StatusAccepted || w.Body.Len() != 0 {
		t.Fatalf("status %d body %q", w.Code, w.Body)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("CORS on 202")
	}
	m := fetchOne(t, e)
	if string(m.Data()) != body {
		t.Fatalf("data %q", m.Data())
	}
	h := m.Headers()
	want := map[string]string{
		queue.HeaderTenant: testTenant, queue.HeaderChannel: testChan, queue.HeaderKey: testKeyID,
		queue.HeaderIdem: "idem-1", "Nats-Msg-Id": testTenant + ":idem-1",
		queue.HeaderReceived: "2026-03-10T12:00:00.123456789Z",
	}
	for k, v := range want {
		if got := h.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, h.Get(queue.HeaderReceived)); err != nil {
		t.Error(err)
	}
	if e.tenantCounts() != [2]int64{1, 1} {
		t.Fatalf("counts %v", e.tenantCounts())
	}
}

func TestIdempotencyKeyGenerated(t *testing.T) {
	e := newEnv(t, envOpts{})
	if w := e.post(goodBody); w.Code != http.StatusAccepted {
		t.Fatalf("status %d", w.Code)
	}
	h := fetchOne(t, e).Headers()
	idem := h.Get(queue.HeaderIdem)
	if len(idem) != 26 || idem != strings.ToLower(idem) {
		t.Fatalf("FB-Idem %q", idem)
	}
	if h.Get("Nats-Msg-Id") != testTenant+":"+idem {
		t.Fatalf("Nats-Msg-Id %q", h.Get("Nats-Msg-Id"))
	}
}

func TestDuplicateStillAccepted(t *testing.T) {
	e := newEnv(t, envOpts{})
	body := `{"message":"x","idempotencyKey":"same"}`
	for i := 0; i < 2; i++ {
		if w := e.post(body); w.Code != http.StatusAccepted {
			t.Fatalf("%d: %d", i, w.Code)
		}
	}
	if n := e.streamMsgs(t); n != 1 {
		t.Fatalf("stream holds %d", n)
	}
	if e.tenantCounts() != [2]int64{1, 1} {
		t.Fatalf("duplicate counted: %v", e.tenantCounts())
	}
}
