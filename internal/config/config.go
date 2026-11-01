// Package config parses FeedBaxter's environment configuration (ARCHITECTURE §11).
package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// Config is the typed, validated configuration.
type Config struct {
	Mode       string // "ingest" | "worker" | "api" | "all"
	HTTPAddr   string
	IngestAddr string
	DataDir    string
	Database   Database
	NATSURL    string // empty = embedded (only valid in mode "all")

	KeyCacheResyncInterval time.Duration
	CounterFlushInterval   time.Duration
	RetentionWindow        time.Duration
	RetentionCheckInterval time.Duration

	DailyLimit, MonthlyLimit int64 // 0 = unlimited
	RateLimitPerKey          int
	RateLimitWindow          time.Duration

	MaxBodyBytes     int64
	MaxMessageChars  int
	MaxMetadataBytes int64

	QueueMaxMsgs    int64
	QueueMaxBytes   int64
	QueueMaxDeliver int

	WorkerConcurrency  int
	WorkerBatchSize    int
	WorkerFetchMaxWait time.Duration
	PublishTimeout     time.Duration

	LogLevel  slog.Level // parsed from debug|info|warn|error
	LogFormat string     // "text" | "json"
}

// Database describes the configured store.
type Database struct {
	Driver, URL, Path string // "sqlite"|"postgres"; DATABASE_URL as given or derived; sqlite file path
}

// Defaults table (ARCHITECTURE §11). Nothing else in this package hard-codes
// a duration or limit.
const (
	defMode                   = "all"
	defHTTPAddr               = ":8080"
	defIngestAddr             = ":8081"
	defDataDir                = "/data"
	defDBFile                 = "feedbaxter.db"
	defKeyCacheResyncInterval = "5m"
	defCounterFlushInterval   = "5s"
	defRetentionWindow        = "17520h"
	defRetentionCheckInterval = "1h"
	defDailyLimit             = "0"
	defMonthlyLimit           = "0"
	defRateLimitPerKey        = "60"
	defRateLimitWindow        = "10s"
	defMaxBodyBytes           = "16384"
	defMaxMessageChars        = "5000"
	defMaxMetadataBytes       = "4096"
	defQueueMaxMsgs           = "100000"
	defQueueMaxBytes          = "268435456"
	defQueueMaxDeliver        = "5"
	defWorkerConcurrencySQL   = 1
	defWorkerConcurrencyPG    = 4
	defWorkerBatchSize        = "500"
	defWorkerFetchMaxWait     = "250ms"
	defPublishTimeout         = "2s"
	defLogLevel               = "info"
	defLogFormat              = "text"
)

var modes = []string{"ingest", "worker", "api", "all"}

// Load reads ./.env (missing is fine), then the process environment, then
// overrides (highest precedence; used for --mode), and parses the result.
// An empty value at any layer counts as unset and falls through to the next.
func Load(overrides map[string]string) (Config, error) {
	dot, err := ReadDotEnv(".env")
	if err != nil {
		return Config{}, err
	}
	return Parse(func(key string) (string, bool) {
		if v, ok := overrides[key]; ok && strings.TrimSpace(v) != "" {
			return v, true
		}
		if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
			return v, true
		}
		if v, ok := dot[key]; ok && strings.TrimSpace(v) != "" {
			return v, true
		}
		return "", false
	})
}

// Parse builds a Config from a lookup function (os.LookupEnv shape). Pure.
// All problems are reported together via errors.Join, one line each.
func Parse(lookup func(key string) (string, bool)) (Config, error) {
	p := parser{lookup: lookup}
	var c Config

	c.Mode = p.str("MODE", defMode)
	if !contains(modes, c.Mode) {
		p.fail("MODE", fmt.Sprintf("%q must be one of %s", c.Mode, strings.Join(modes, ", ")))
	}
	c.HTTPAddr = p.str("HTTP_ADDR", defHTTPAddr)
	c.IngestAddr = p.str("INGEST_ADDR", defIngestAddr)
	c.DataDir = p.str("DATA_DIR", defDataDir)
	c.Database = p.database(c.DataDir)

	c.NATSURL, _ = p.get("NATS_URL")
	if c.NATSURL == "" && contains(modes, c.Mode) && c.Mode != "all" {
		p.fail("NATS_URL", fmt.Sprintf("required when MODE is %s (only MODE=all can embed NATS)", c.Mode))
	}

	c.KeyCacheResyncInterval = p.duration("KEY_CACHE_RESYNC_INTERVAL", defKeyCacheResyncInterval)
	c.CounterFlushInterval = p.duration("COUNTER_FLUSH_INTERVAL", defCounterFlushInterval)
	c.RetentionWindow = p.duration("RETENTION_WINDOW", defRetentionWindow)
	c.RetentionCheckInterval = p.duration("RETENTION_CHECK_INTERVAL", defRetentionCheckInterval)

	c.DailyLimit = p.int64("DAILY_LIMIT", defDailyLimit, 0)
	c.MonthlyLimit = p.int64("MONTHLY_LIMIT", defMonthlyLimit, 0)
	c.RateLimitPerKey = int(p.int64("RATE_LIMIT_PER_KEY", defRateLimitPerKey, 1))
	c.RateLimitWindow = p.duration("RATE_LIMIT_WINDOW", defRateLimitWindow)

	c.MaxBodyBytes = p.int64("MAX_BODY_BYTES", defMaxBodyBytes, 1)
	c.MaxMessageChars = int(p.int64("MAX_MESSAGE_CHARS", defMaxMessageChars, 1))
	c.MaxMetadataBytes = p.int64("MAX_METADATA_BYTES", defMaxMetadataBytes, 1)
	if p.valid("MAX_BODY_BYTES") && p.valid("MAX_METADATA_BYTES") && c.MaxMetadataBytes >= c.MaxBodyBytes {
		p.fail("MAX_METADATA_BYTES", fmt.Sprintf("%d must be less than MAX_BODY_BYTES (%d)", c.MaxMetadataBytes, c.MaxBodyBytes))
	}

	c.QueueMaxMsgs = p.int64("QUEUE_MAX_MSGS", defQueueMaxMsgs, 1)
	c.QueueMaxBytes = p.int64("QUEUE_MAX_BYTES", defQueueMaxBytes, 1)
	c.QueueMaxDeliver = int(p.int64("QUEUE_MAX_DELIVER", defQueueMaxDeliver, 1))

	wcDefault := ""
	switch c.Database.Driver {
	case "sqlite":
		wcDefault = strconv.Itoa(defWorkerConcurrencySQL)
	case "postgres":
		wcDefault = strconv.Itoa(defWorkerConcurrencyPG)
	}
	if wcDefault == "" {
		// Database is invalid and already reported; still validate an explicit value.
		wcDefault = strconv.Itoa(defWorkerConcurrencySQL)
	}
	c.WorkerConcurrency = int(p.int64("WORKER_CONCURRENCY", wcDefault, 1))
	c.WorkerBatchSize = int(p.int64("WORKER_BATCH_SIZE", defWorkerBatchSize, 1))
	c.WorkerFetchMaxWait = p.duration("WORKER_FETCH_MAX_WAIT", defWorkerFetchMaxWait)
	c.PublishTimeout = p.duration("PUBLISH_TIMEOUT", defPublishTimeout)

	levelStr := strings.ToLower(p.str("LOG_LEVEL", defLogLevel))
	switch levelStr {
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "info":
		c.LogLevel = slog.LevelInfo
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		p.fail("LOG_LEVEL", fmt.Sprintf("%q must be one of debug, info, warn, error", levelStr))
	}
	c.LogFormat = strings.ToLower(p.str("LOG_FORMAT", defLogFormat))
	if c.LogFormat != "text" && c.LogFormat != "json" {
		p.fail("LOG_FORMAT", fmt.Sprintf("%q must be one of text, json", c.LogFormat))
	}

	if len(p.errs) > 0 {
		return Config{}, errors.Join(p.errs...)
	}
	return c, nil
}

// Logger returns a slog.Logger honouring LogLevel and LogFormat, writing to w.
func (c Config) Logger(w io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{Level: c.LogLevel}
	if c.LogFormat == "json" {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}

type parser struct {
	lookup func(string) (string, bool)
	errs   []error
	failed map[string]bool
}

func (p *parser) get(key string) (string, bool) {
	v, ok := p.lookup(key)
	v = strings.TrimSpace(v)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func (p *parser) fail(key, msg string) {
	if p.failed == nil {
		p.failed = map[string]bool{}
	}
	p.failed[key] = true
	p.errs = append(p.errs, fmt.Errorf("%s: %s", key, msg))
}

func (p *parser) valid(key string) bool { return !p.failed[key] }

func (p *parser) str(key, def string) string {
	if v, ok := p.get(key); ok {
		return v
	}
	return def
}

func (p *parser) duration(key, def string) time.Duration {
	raw := p.str(key, def)
	d, err := time.ParseDuration(raw)
	if err != nil {
		p.fail(key, fmt.Sprintf("%q is not a Go duration (examples: 90s, 5m, 17520h)", raw))
		return 0
	}
	if d <= 0 {
		p.fail(key, fmt.Sprintf("%q must be greater than zero", raw))
		return 0
	}
	return d
}

func (p *parser) int64(key, def string, min int64) int64 {
	raw := p.str(key, def)
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		p.fail(key, fmt.Sprintf("%q is not a whole number", raw))
		return 0
	}
	if n < min {
		if min == 0 {
			p.fail(key, fmt.Sprintf("%q must be 0 or greater", raw))
		} else {
			p.fail(key, fmt.Sprintf("%q must be greater than zero", raw))
		}
		return 0
	}
	return n
}

// database resolves DATABASE_URL. Error messages never contain the URL
// beyond its scheme, so a password cannot leak.
func (p *parser) database(dataDir string) Database {
	raw, set := p.get("DATABASE_URL")
	if !set {
		file := path.Join(dataDir, defDBFile)
		return Database{Driver: "sqlite", URL: "sqlite://" + file, Path: file}
	}
	const accepted = "use sqlite://<path> or postgres://..."
	switch {
	case strings.HasPrefix(raw, "sqlite://"):
		rest := strings.TrimPrefix(raw, "sqlite://")
		if strings.Contains(rest, "?") {
			p.fail("DATABASE_URL", "a query string is not allowed on sqlite URLs (SQLite pragmas are set by FeedBaxter; remove the query string)")
			return Database{}
		}
		if rest == "" {
			p.fail("DATABASE_URL", "sqlite URL has no file path (example: sqlite:///data/feedbaxter.db)")
			return Database{}
		}
		return Database{Driver: "sqlite", URL: raw, Path: rest}
	case strings.HasPrefix(raw, "postgres://"), strings.HasPrefix(raw, "postgresql://"):
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			p.fail("DATABASE_URL", "postgres URL is malformed or has no host (example: postgres://user:pass@host:5432/dbname)")
			return Database{}
		}
		return Database{Driver: "postgres", URL: raw}
	}
	scheme, _, found := strings.Cut(raw, "://")
	if !found {
		p.fail("DATABASE_URL", "value has no scheme; "+accepted)
	} else {
		p.fail("DATABASE_URL", fmt.Sprintf("scheme %q is not supported; %s", scheme, accepted))
	}
	return Database{}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
