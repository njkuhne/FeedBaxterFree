package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func mustParse(t *testing.T, m map[string]string) Config {
	t.Helper()
	c, err := Parse(env(m))
	if err != nil {
		t.Fatalf("Parse(%v): %v", m, err)
	}
	return c
}

func parseErr(t *testing.T, m map[string]string) string {
	t.Helper()
	_, err := Parse(env(m))
	if err == nil {
		t.Fatalf("Parse(%v): expected error", m)
	}
	return err.Error()
}

func TestDefaults(t *testing.T) {
	c := mustParse(t, nil)
	want := Config{
		Mode: "all", HTTPAddr: ":8080", IngestAddr: ":8081", DataDir: "/data",
		Database:               Database{Driver: "sqlite", URL: "sqlite:///data/feedbaxter.db", Path: "/data/feedbaxter.db"},
		KeyCacheResyncInterval: 5 * time.Minute, CounterFlushInterval: 5 * time.Second,
		RetentionWindow: 17520 * time.Hour, RetentionCheckInterval: time.Hour,
		RateLimitPerKey: 60, RateLimitWindow: 10 * time.Second,
		MaxBodyBytes: 16384, MaxMessageChars: 5000, MaxMetadataBytes: 4096,
		QueueMaxMsgs: 100000, QueueMaxBytes: 268435456, QueueMaxDeliver: 5,
		WorkerConcurrency: 1, WorkerBatchSize: 500, WorkerFetchMaxWait: 250 * time.Millisecond,
		PublishTimeout: 2 * time.Second, LogLevel: slog.LevelInfo, LogFormat: "text",
	}
	if c != want {
		t.Errorf("defaults mismatch:\n got %+v\nwant %+v", c, want)
	}
}

func TestEveryVariableOverrides(t *testing.T) {
	c := mustParse(t, map[string]string{
		"MODE": "worker", "HTTP_ADDR": ":9000", "INGEST_ADDR": ":9001", "DATA_DIR": "/srv/x",
		"DATABASE_URL": "postgres://u:p@h:5432/d", "NATS_URL": "nats://n:4222",
		"KEY_CACHE_RESYNC_INTERVAL": "7m", "COUNTER_FLUSH_INTERVAL": "9s",
		"RETENTION_WINDOW": "24h", "RETENTION_CHECK_INTERVAL": "30m",
		"DAILY_LIMIT": "11", "MONTHLY_LIMIT": "22",
		"RATE_LIMIT_PER_KEY": "7", "RATE_LIMIT_WINDOW": "3s",
		"MAX_BODY_BYTES": "9000", "MAX_MESSAGE_CHARS": "123", "MAX_METADATA_BYTES": "800",
		"QUEUE_MAX_MSGS": "10", "QUEUE_MAX_BYTES": "20", "QUEUE_MAX_DELIVER": "3",
		"WORKER_CONCURRENCY": "6", "WORKER_BATCH_SIZE": "40", "WORKER_FETCH_MAX_WAIT": "1s",
		"PUBLISH_TIMEOUT": "4s", "LOG_LEVEL": "debug", "LOG_FORMAT": "json",
	})
	want := Config{
		Mode: "worker", HTTPAddr: ":9000", IngestAddr: ":9001", DataDir: "/srv/x",
		Database: Database{Driver: "postgres", URL: "postgres://u:p@h:5432/d"}, NATSURL: "nats://n:4222",
		KeyCacheResyncInterval: 7 * time.Minute, CounterFlushInterval: 9 * time.Second,
		RetentionWindow: 24 * time.Hour, RetentionCheckInterval: 30 * time.Minute,
		DailyLimit: 11, MonthlyLimit: 22, RateLimitPerKey: 7, RateLimitWindow: 3 * time.Second,
		MaxBodyBytes: 9000, MaxMessageChars: 123, MaxMetadataBytes: 800,
		QueueMaxMsgs: 10, QueueMaxBytes: 20, QueueMaxDeliver: 3,
		WorkerConcurrency: 6, WorkerBatchSize: 40, WorkerFetchMaxWait: time.Second,
		PublishTimeout: 4 * time.Second, LogLevel: slog.LevelDebug, LogFormat: "json",
	}
	if c != want {
		t.Errorf("override mismatch:\n got %+v\nwant %+v", c, want)
	}
}

func TestDurationsAreGoDurations(t *testing.T) {
	vars := []string{"KEY_CACHE_RESYNC_INTERVAL", "COUNTER_FLUSH_INTERVAL", "RETENTION_WINDOW",
		"RETENTION_CHECK_INTERVAL", "RATE_LIMIT_WINDOW", "WORKER_FETCH_MAX_WAIT", "PUBLISH_TIMEOUT"}
	for _, v := range vars {
		for _, good := range []string{"90s", "1h30m"} {
			mustParse(t, map[string]string{v: good})
		}
		for _, bad := range []string{"2y", "5", "-1s", "0s"} {
			msg := parseErr(t, map[string]string{v: bad})
			if !strings.Contains(msg, v+":") || !strings.Contains(msg, `"`+bad+`"`) {
				t.Errorf("%s=%s: message %q lacks var name or value", v, bad, msg)
			}
		}
	}
	c := mustParse(t, map[string]string{"RETENTION_WINDOW": "1h30m"})
	if c.RetentionWindow != 90*time.Minute {
		t.Errorf("RetentionWindow = %v", c.RetentionWindow)
	}
}

func TestIntegersValidated(t *testing.T) {
	cases := []struct {
		key, val string
		ok       bool
	}{
		{"DAILY_LIMIT", "0", true}, {"DAILY_LIMIT", "-1", false}, {"DAILY_LIMIT", "abc", false}, {"DAILY_LIMIT", "1.5", false},
		{"MONTHLY_LIMIT", "0", true}, {"MONTHLY_LIMIT", "-5", false}, {"MONTHLY_LIMIT", "x", false},
		{"RATE_LIMIT_PER_KEY", "0", false}, {"RATE_LIMIT_PER_KEY", "1", true}, {"RATE_LIMIT_PER_KEY", "ten", false},
		{"MAX_BODY_BYTES", "0", false}, {"MAX_BODY_BYTES", "16kb", false},
		{"MAX_MESSAGE_CHARS", "0", false}, {"MAX_MESSAGE_CHARS", "-3", false},
		{"MAX_METADATA_BYTES", "0", false}, {"MAX_METADATA_BYTES", "4095", true},
		{"QUEUE_MAX_MSGS", "0", false}, {"QUEUE_MAX_BYTES", "-1", false}, {"QUEUE_MAX_DELIVER", "0", false},
		{"WORKER_CONCURRENCY", "0", false}, {"WORKER_CONCURRENCY", "2", true},
		{"WORKER_BATCH_SIZE", "0", false}, {"WORKER_BATCH_SIZE", "n", false},
	}
	for _, tc := range cases {
		_, err := Parse(env(map[string]string{tc.key: tc.val}))
		if tc.ok && err != nil {
			t.Errorf("%s=%s: unexpected error %v", tc.key, tc.val, err)
		}
		if !tc.ok {
			if err == nil {
				t.Errorf("%s=%s: expected error", tc.key, tc.val)
			} else if !strings.Contains(err.Error(), tc.key+":") {
				t.Errorf("%s=%s: error %q lacks var name", tc.key, tc.val, err)
			}
		}
	}
	// Metadata must be smaller than the body.
	msg := parseErr(t, map[string]string{"MAX_BODY_BYTES": "1000", "MAX_METADATA_BYTES": "1000"})
	if !strings.Contains(msg, "MAX_METADATA_BYTES") {
		t.Errorf("metadata >= body message: %q", msg)
	}
}

func TestAllErrorsReported(t *testing.T) {
	msg := parseErr(t, map[string]string{"MODE": "nope", "RETENTION_WINDOW": "2y", "DAILY_LIMIT": "-1"})
	for _, v := range []string{"MODE", "RETENTION_WINDOW", "DAILY_LIMIT"} {
		if !strings.Contains(msg, v+":") {
			t.Errorf("message lacks %s: %q", v, msg)
		}
	}
	if n := strings.Count(msg, "\n"); n != 2 {
		t.Errorf("expected 3 lines, got %d newlines in %q", n, msg)
	}
}

func TestModeValidation(t *testing.T) {
	for _, m := range []string{"ingest", "worker", "api", "all"} {
		mustParse(t, map[string]string{"MODE": m, "NATS_URL": "nats://x"})
	}
	msg := parseErr(t, map[string]string{"MODE": "nope"})
	if !strings.Contains(msg, `MODE: "nope" must be one of ingest, worker, api, all`) {
		t.Errorf("message = %q", msg)
	}
}

func TestNATSURLRequiredOutsideAll(t *testing.T) {
	for _, m := range []string{"worker", "api", "ingest"} {
		msg := parseErr(t, map[string]string{"MODE": m})
		want := "NATS_URL: required when MODE is " + m + " (only MODE=all can embed NATS)"
		if !strings.Contains(msg, want) {
			t.Errorf("mode %s: message %q", m, msg)
		}
	}
	mustParse(t, map[string]string{"MODE": "all"})
	c := mustParse(t, map[string]string{"MODE": "all", "NATS_URL": "nats://x:4222"})
	if c.NATSURL != "nats://x:4222" {
		t.Errorf("NATSURL = %q", c.NATSURL)
	}
}

func TestDatabaseURLDefaultFollowsDataDir(t *testing.T) {
	c := mustParse(t, map[string]string{"DATA_DIR": "/srv/fb"})
	want := Database{Driver: "sqlite", URL: "sqlite:///srv/fb/feedbaxter.db", Path: "/srv/fb/feedbaxter.db"}
	if c.Database != want {
		t.Errorf("got %+v want %+v", c.Database, want)
	}
}

func TestDatabaseURLSQLite(t *testing.T) {
	good := map[string]string{
		"sqlite:///data/x.db": "/data/x.db",
		"sqlite://./x.db":     "./x.db",
	}
	for in, path := range good {
		c := mustParse(t, map[string]string{"DATABASE_URL": in})
		if c.Database != (Database{Driver: "sqlite", URL: in, Path: path}) {
			t.Errorf("%s: got %+v", in, c.Database)
		}
	}
	for _, bad := range []string{"sqlite://", "sqlite:///data/x.db?_pragma=foo"} {
		msg := parseErr(t, map[string]string{"DATABASE_URL": bad})
		if !strings.Contains(msg, "DATABASE_URL:") {
			t.Errorf("%s: message %q", bad, msg)
		}
	}
	if msg := parseErr(t, map[string]string{"DATABASE_URL": "sqlite://x.db?a=b"}); !strings.Contains(msg, "remove the query string") {
		t.Errorf("message = %q", msg)
	}
}

func TestDatabaseURLPostgres(t *testing.T) {
	for _, in := range []string{"postgres://u:p@h:5432/db?sslmode=disable", "postgresql://u@h/db"} {
		c := mustParse(t, map[string]string{"DATABASE_URL": in})
		if c.Database.Driver != "postgres" || c.Database.URL != in {
			t.Errorf("%s: got %+v", in, c.Database)
		}
	}
	msg := parseErr(t, map[string]string{"DATABASE_URL": "postgres://u:hunter2@/db"})
	if strings.Contains(msg, "hunter2") {
		t.Errorf("message leaks password: %q", msg)
	}
}

func TestDatabaseURLUnknownSchemeHidesPassword(t *testing.T) {
	for _, in := range []string{"mysql://u:secret@h/db", "u:secret@h/db"} {
		msg := parseErr(t, map[string]string{"DATABASE_URL": in})
		if strings.Contains(msg, "secret") || strings.Contains(msg, in) {
			t.Errorf("%s: message leaks URL: %q", in, msg)
		}
	}
	if msg := parseErr(t, map[string]string{"DATABASE_URL": "mysql://u:secret@h/db"}); !strings.Contains(msg, "mysql") {
		t.Errorf("message lacks scheme: %q", msg)
	}
}

func TestWorkerConcurrencyDefaultByDialect(t *testing.T) {
	pg := "postgres://u:p@h/db"
	cases := []struct {
		m    map[string]string
		want int
	}{
		{nil, 1},
		{map[string]string{"DATABASE_URL": pg}, 4},
		{map[string]string{"WORKER_CONCURRENCY": "3"}, 3},
		{map[string]string{"DATABASE_URL": pg, "WORKER_CONCURRENCY": "2"}, 2},
	}
	for _, tc := range cases {
		if got := mustParse(t, tc.m).WorkerConcurrency; got != tc.want {
			t.Errorf("%v: got %d want %d", tc.m, got, tc.want)
		}
	}
}

func TestLogLevelAndFormat(t *testing.T) {
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "INFO": slog.LevelInfo, "Warn": slog.LevelWarn, "error": slog.LevelError}
	for in, want := range levels {
		if got := mustParse(t, map[string]string{"LOG_LEVEL": in}).LogLevel; got != want {
			t.Errorf("LOG_LEVEL=%s: got %v", in, got)
		}
	}
	for _, f := range []string{"text", "JSON"} {
		mustParse(t, map[string]string{"LOG_FORMAT": f})
	}
	for _, m := range []map[string]string{{"LOG_LEVEL": "trace"}, {"LOG_FORMAT": "xml"}} {
		parseErr(t, m)
	}

	var buf bytes.Buffer
	mustParse(t, map[string]string{"LOG_FORMAT": "json"}).Logger(&buf).Info("hi")
	if !strings.HasPrefix(buf.String(), "{") {
		t.Errorf("json logger output = %q", buf.String())
	}
	buf.Reset()
	mustParse(t, nil).Logger(&buf).Info("hi")
	if strings.HasPrefix(buf.String(), "{") || buf.Len() == 0 {
		t.Errorf("text logger output = %q", buf.String())
	}
	buf.Reset()
	mustParse(t, map[string]string{"LOG_LEVEL": "error"}).Logger(&buf).Info("hi")
	if buf.Len() != 0 {
		t.Errorf("info logged at error level: %q", buf.String())
	}
}

func TestLoadPrecedence(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("MODE=worker\nNATS_URL=nats://x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODE", "api")

	c, err := Load(map[string]string{"MODE": "ingest"})
	if err != nil || c.Mode != "ingest" {
		t.Errorf("override: mode=%q err=%v", c.Mode, err)
	}
	c, err = Load(nil)
	if err != nil || c.Mode != "api" {
		t.Errorf("env: mode=%q err=%v", c.Mode, err)
	}
	// Setenv registered cleanup; unset now so .env applies.
	os.Unsetenv("MODE")
	c, err = Load(nil)
	if err != nil || c.Mode != "worker" || c.NATSURL != "nats://x" {
		t.Errorf("dotenv: %+v err=%v", c, err)
	}
}
