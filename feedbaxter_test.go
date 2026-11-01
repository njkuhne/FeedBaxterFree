package feedbaxter

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/store"
)

// isolate points the config at a temp SQLite file and an empty working directory.
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "fb.db"))
}

const waitTimeout = 10 * time.Second

// syncBuffer is a log sink that is safe to read while Run writes.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	b := &syncBuffer{}
	old := logOutput
	logOutput = b
	t.Cleanup(func() { logOutput = old })
	return b
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// isolateAll is isolate plus a DATA_DIR in the same directory; it returns the SQLite path.
func isolateAll(t *testing.T) string {
	t.Helper()
	isolate(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_DIR", dir)
	t.Setenv("NATS_URL", "")
	return filepath.Join(dir, "fb.db")
}

type running struct {
	addr   string
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	err    error
}

// startRun runs Run on a fresh 127.0.0.1 port. The test fails if Run is still going 5s after cleanup.
func startRun(t *testing.T, opts Options) *running {
	t.Helper()
	r := &running{addr: freeAddr(t), done: make(chan error, 1)}
	t.Setenv("HTTP_ADDR", r.addr)
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { r.done <- Run(ctx, opts) }()
	t.Cleanup(func() { r.stop(t) })
	return r
}

// stop cancels Run and returns its error; Run must return within 5s.
func (r *running) stop(t *testing.T) error {
	t.Helper()
	r.once.Do(func() {
		r.cancel()
		select {
		case r.err = <-r.done:
		case <-time.After(5 * time.Second):
			t.Error("Run did not return within 5s of cancel")
		}
	})
	return r.err
}

func (r *running) url(path string) string { return "http://" + r.addr + path }

// waitReady polls /readyz until it answers 200.
func (r *running) waitReady(t *testing.T) {
	t.Helper()
	deadline := time.After(waitTimeout)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case err := <-r.done:
			t.Fatalf("Run returned before ready: %v", err)
		case <-deadline:
			t.Fatal("timed out waiting for /readyz")
		case <-tick.C:
			resp, err := http.Get(r.url("/readyz"))
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
	}
}

func (r *running) do(t *testing.T, method, key, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, r.url("/v1/feedback"), strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

var keyLine = regexp.MustCompile(`ingest key for channel Default: (fbx_[a-z2-7]{32})`)

func logKey(t *testing.T, log *syncBuffer) string {
	t.Helper()
	m := keyLine.FindStringSubmatch(log.String())
	if m == nil {
		t.Fatalf("no ingest key in log:\n%s", log.String())
	}
	return m[1]
}

func TestRunAllEndToEnd(t *testing.T) {
	dbPath := isolateAll(t)
	log := captureLog(t)
	r := startRun(t, Options{Mode: "all"})
	r.waitReady(t)

	resp, err := http.Get(r.url("/healthz"))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("/healthz = %v, %v", resp, err)
	}
	resp.Body.Close()

	if n := len(keyLine.FindAllString(log.String(), -1)); n != 1 {
		t.Fatalf("key logged %d times, want 1", n)
	}
	key := logKey(t, log)
	if got := r.do(t, http.MethodPost, key, `{"message":"hi"}`).StatusCode; got != http.StatusAccepted {
		t.Fatalf("POST = %d, want 202", got)
	}
	if got := r.do(t, http.MethodOptions, "", "").StatusCode; got != http.StatusNoContent {
		t.Fatalf("OPTIONS = %d, want 204", got)
	}
	if got := r.do(t, http.MethodPost, "fbx_wrong", `{"message":"hi"}`).StatusCode; got != http.StatusUnauthorized {
		t.Fatalf("bad key = %d, want 401", got)
	}

	db, err := store.Open(context.Background(), config.Database{Driver: "sqlite", Path: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	deadline := time.Now().Add(waitTimeout)
	for {
		var msg string
		err := db.SQL.QueryRow("SELECT message FROM feedback").Scan(&msg)
		if err == nil {
			if msg != "hi" {
				t.Fatalf("message = %q, want hi", msg)
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) || time.Now().After(deadline) {
			t.Fatalf("waiting for the feedback row: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.stop(t); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}

func TestRunAllRestartKeepsKey(t *testing.T) {
	isolateAll(t)
	log := captureLog(t)
	first := startRun(t, Options{Mode: "all"})
	first.waitReady(t)
	key := logKey(t, log)
	if err := first.stop(t); err != nil {
		t.Fatalf("first Run = %v", err)
	}

	before := log.String()
	second := startRun(t, Options{Mode: "all"})
	second.waitReady(t)
	if strings.Contains(log.String()[len(before):], "ingest key for channel") {
		t.Fatalf("key logged again on restart:\n%s", log.String())
	}
	if got := second.do(t, http.MethodPost, key, `{"message":"again"}`).StatusCode; got != http.StatusAccepted {
		t.Fatalf("POST with first key = %d, want 202", got)
	}
	if err := second.stop(t); err != nil {
		t.Fatalf("second Run = %v", err)
	}
}

func TestRunBindError(t *testing.T) {
	isolateAll(t)
	captureLog(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	t.Setenv("HTTP_ADDR", ln.Addr().String())
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), Options{Mode: "all"}) }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "listen") {
			t.Fatalf("Run = %v, want a listen error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after a bind error")
	}
}

func TestRunOnStartErrorAborts(t *testing.T) {
	isolateAll(t)
	captureLog(t)
	t.Setenv("HTTP_ADDR", freeAddr(t))
	boom := errors.New("boom")
	hook := func(_ context.Context, d ext.Deps) error {
		if d.DB == nil || d.Dialect != "sqlite" || d.Keys == nil || d.Logger == nil {
			t.Errorf("hook deps = %+v", d)
		}
		return boom
	}
	done := make(chan error, 1)
	go func() {
		done <- Run(context.Background(), Options{Mode: "all", OnStart: []func(context.Context, ext.Deps) error{hook}})
	}()
	select {
	case err := <-done:
		if !errors.Is(err, boom) {
			t.Fatalf("Run = %v, want boom", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return after a hook error")
	}
}

func TestRunIngestModeNeedsNATS(t *testing.T) {
	isolateAll(t)
	err := Run(context.Background(), Options{Mode: "ingest"})
	if err == nil || !strings.Contains(err.Error(), "NATS_URL") {
		t.Fatalf("Run error = %v, want NATS_URL config error", err)
	}
}

func TestRunRejectsBadMode(t *testing.T) {
	isolate(t)
	err := Run(context.Background(), Options{Mode: "nope"})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Run error = %v, want bad-mode error naming nope", err)
	}
}

func TestMigrateSQLite(t *testing.T) {
	isolate(t)
	got, err := Migrate(context.Background(), Options{})
	if err != nil || !reflect.DeepEqual(got, []string{"0001_core", "0002_triage"}) {
		t.Fatalf("first Migrate = %v, %v; want [0001_core 0002_triage]", got, err)
	}
	got, err = Migrate(context.Background(), Options{})
	if err != nil || len(got) != 0 {
		t.Fatalf("second Migrate = %v, %v; want none", got, err)
	}
}

func TestMigrateWithExtension(t *testing.T) {
	isolate(t)
	m := ext.Migration{
		Version:  "ext_0001_demo",
		SQLite:   "CREATE TABLE ext_demo (id INTEGER PRIMARY KEY)",
		Postgres: "CREATE TABLE ext_demo (id BIGINT PRIMARY KEY)",
	}
	got, err := Migrate(context.Background(), Options{Migrations: []ext.Migration{m}})
	if err != nil || !reflect.DeepEqual(got, []string{"0001_core", "0002_triage", "ext_0001_demo"}) {
		t.Fatalf("Migrate = %v, %v; want [0001_core 0002_triage ext_0001_demo]", got, err)
	}
}

// A connection that never sends a request must not hold up shutdown
// (http.Server.Shutdown alone waits 5s for those).
func TestRunShutdownWithUnusedConn(t *testing.T) {
	isolateAll(t)
	captureLog(t)
	r := startRun(t, Options{Mode: "all"})
	r.waitReady(t)
	c, err := net.Dial("tcp", r.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	r.cancel()
	select {
	case err := <-r.done:
		r.once.Do(func() {})
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2s with an unused connection open")
	}
}

var adminLine = regexp.MustCompile(`admin login admin@localhost temporary password: ([A-Za-z2-9]{20}) `)

// TestRunAllAdminRoundTrip boots all mode and drives the whole self-host path:
// first-boot admin, forced password change, channel and key creation through
// the API, ingest, the inbox, and key revocation reaching ingest.
func TestRunAllAdminRoundTrip(t *testing.T) {
	isolateAll(t)
	log := captureLog(t)
	r := startRun(t, Options{Mode: "all"})
	r.waitReady(t)

	m := adminLine.FindStringSubmatch(log.String())
	if m == nil || len(adminLine.FindAllString(log.String(), -1)) != 1 {
		t.Fatalf("want exactly one admin line in the log:\n%s", log.String())
	}
	temp := m[1]

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	call := func(method, path, body string) (int, map[string]any) {
		t.Helper()
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, r.url(path), rd)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Requested-With", "test")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		return resp.StatusCode, out
	}

	if code, _ := call("POST", "/api/auth/login", `{"email":"admin@localhost","password":"`+temp+`"}`); code != 200 {
		t.Fatalf("login = %d", code)
	}
	if code, body := call("GET", "/api/feedback", ""); code != 403 || body["error"] != "password_change_required" {
		t.Fatalf("feedback before change = %d %v", code, body)
	}
	const newPW = "a much better password"
	if code, body := call("POST", "/api/auth/password", `{"currentPassword":"`+temp+`","newPassword":"`+newPW+`"}`); code != 204 {
		t.Fatalf("change password = %d %v", code, body)
	}
	code, ch := call("POST", "/api/channels", `{"name":"Web"}`)
	if code != 201 {
		t.Fatalf("create channel = %d %v", code, ch)
	}
	code, k := call("POST", "/api/channels/"+ch["id"].(string)+"/keys", "")
	if code != 201 {
		t.Fatalf("create key = %d %v", code, k)
	}
	secret, keyID := k["secret"].(string), k["key"].(map[string]any)["id"].(string)
	if strings.Contains(log.String(), secret) || strings.Contains(log.String(), newPW) {
		t.Fatalf("a secret reached the log:\n%s", log.String())
	}

	// A key made through the API reaches ingest through the key cache.
	deadline := time.Now().Add(waitTimeout)
	for {
		if got := r.do(t, http.MethodPost, secret, `{"message":"hello from the API test"}`).StatusCode; got == http.StatusAccepted {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the new key never became valid at ingest")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for {
		code, list := call("GET", "/api/feedback", "")
		if code != 200 {
			t.Fatalf("list = %d %v", code, list)
		}
		items := list["items"].([]any)
		if len(items) == 1 && items[0].(map[string]any)["message"] == "hello from the API test" &&
			items[0].(map[string]any)["channelName"] == "Web" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the feedback never showed up: %v", list)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if code, body := call("POST", "/api/keys/"+keyID+"/revoke", ""); code != 200 || body["revokedAt"] == nil {
		t.Fatalf("revoke = %d %v", code, body)
	}
	revokeBy := time.Now().Add(5 * time.Second)
	for {
		got := r.do(t, http.MethodPost, secret, `{"message":"after revoke"}`).StatusCode
		if got == http.StatusUnauthorized {
			break
		}
		if time.Now().After(revokeBy) {
			t.Fatalf("revoked key still answers %d after 5s", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := r.stop(t); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
}
