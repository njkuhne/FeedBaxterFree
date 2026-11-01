package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/selfhost"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

type noKeys struct{}

func (noKeys) SyncKey(context.Context, string) error    { return nil }
func (noKeys) SyncTenant(context.Context, string) error { return nil }

func do(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := do("version")
	if code != 0 || strings.TrimSpace(out) != "feedbaxter dev" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errs := do("bogus")
	if code != 2 || !strings.Contains(errs, "Usage:") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestNoCommand(t *testing.T) {
	code, _, errs := do()
	if code != 2 || !strings.Contains(errs, "Usage:") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestServeBadMode(t *testing.T) {
	isolate(t)
	code, _, errs := do("serve", "--mode=nope")
	if code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "fb.db"))
}

func TestMigrateCommand(t *testing.T) {
	isolate(t)
	code, out, errs := do("migrate")
	if code != 0 || !strings.Contains(out, "applied 0001_core") {
		t.Fatalf("first: code=%d out=%q stderr=%q", code, out, errs)
	}
	code, out, errs = do("migrate")
	if code != 0 || !strings.Contains(out, "schema is up to date") {
		t.Fatalf("second: code=%d out=%q stderr=%q", code, out, errs)
	}
}

func TestResetAdminWithoutOrganisation(t *testing.T) {
	isolate(t)
	code, out, errs := do("reset-admin")
	if code != 1 || out != "" || !strings.Contains(errs, "start the server once first") {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, errs)
	}
}

func TestResetAdminCommand(t *testing.T) {
	isolate(t)
	if code, _, errs := do("migrate"); code != 0 {
		t.Fatalf("migrate: %d %s", code, errs)
	}
	db, err := store.Open(context.Background(), config.Database{Driver: "sqlite", Path: "fb.db"})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := selfhost.Bootstrap(context.Background(), db, noKeys{}, log, time.Now(), "root@example.com"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	want := regexp.MustCompile(`^temporary password for root@example\.com: [A-Za-z2-9]{20}\n$`)
	if code, out, errs := do("reset-admin"); code != 0 || !want.MatchString(out) {
		t.Fatalf("reset-admin: code=%d out=%q stderr=%q", code, out, errs)
	}
	if code, out, errs := do("reset-admin", "--email=ROOT@example.com"); code != 0 || !want.MatchString(out) {
		t.Fatalf("reset-admin --email: code=%d out=%q stderr=%q", code, out, errs)
	}
	if code, _, errs := do("reset-admin", "--email=nobody@example.com"); code != 1 || errs == "" {
		t.Fatalf("unknown email: code=%d stderr=%q", code, errs)
	}
	if code, _, _ := do("reset-admin", "--bogus"); code != 2 {
		t.Fatalf("bad flag: code=%d, want 2", code)
	}
	if code, _, _ := do("reset-admin", "extra"); code != 2 {
		t.Fatalf("extra arg: code=%d, want 2", code)
	}
}

func TestReadyURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{":8080", "http://127.0.0.1:8080/readyz"},
		{"0.0.0.0:8080", "http://127.0.0.1:8080/readyz"},
		{"[::]:8080", "http://127.0.0.1:8080/readyz"},
		{"127.0.0.1:9000", "http://127.0.0.1:9000/readyz"},
		{"10.0.0.5:9000", "http://10.0.0.5:9000/readyz"},
		{"[::1]:8080", "http://[::1]:8080/readyz"},
	}
	for _, c := range cases {
		got, err := readyURL(c.in)
		if err != nil || got != c.want {
			t.Errorf("readyURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if got, err := readyURL("nonsense"); err == nil {
		t.Errorf("readyURL(nonsense) = %q, want error", got)
	}
}

func statusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCheckReady(t *testing.T) {
	ctx := context.Background()
	if err := checkReady(ctx, statusServer(t, 200).URL+"/readyz"); err != nil {
		t.Errorf("200: %v", err)
	}
	if err := checkReady(ctx, statusServer(t, 503).URL+"/readyz"); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("503: err=%v", err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	url := closed.URL + "/readyz"
	closed.Close()
	if err := checkReady(ctx, url); err == nil {
		t.Error("closed server: want error")
	}
}

func TestHealthcheckCommand(t *testing.T) {
	isolate(t)
	ok := statusServer(t, 200)
	t.Setenv("HTTP_ADDR", strings.TrimPrefix(ok.URL, "http://"))
	if code, out, errs := do("healthcheck"); code != 0 || out != "" || errs != "" {
		t.Fatalf("200: code=%d out=%q stderr=%q", code, out, errs)
	}

	bad := statusServer(t, 503)
	t.Setenv("HTTP_ADDR", strings.TrimPrefix(bad.URL, "http://"))
	code, out, errs := do("healthcheck")
	if code != 1 || out != "" || strings.Count(errs, "\n") != 1 || !strings.Contains(errs, "503") {
		t.Fatalf("503: code=%d out=%q stderr=%q", code, out, errs)
	}

	if code, _, _ := do("healthcheck", "extra"); code != 2 {
		t.Fatalf("extra arg: code=%d, want 2", code)
	}
}

func TestHelpMentionsHealthcheck(t *testing.T) {
	if code, out, _ := do("help"); code != 0 || !strings.Contains(out, "healthcheck") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}
