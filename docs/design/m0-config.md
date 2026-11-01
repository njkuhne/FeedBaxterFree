# M0 brief: `internal/config`

Author: Opus director · Milestone M0 · Source: ARCHITECTURE §11, CLAUDE.md (durations from config)

## Goal

Every variable in ARCHITECTURE §11 parsed from the environment (plus `.env` if present; env wins)
into a typed `Config`, with defaults, Go durations and validation that reports **all** problems at
once in plain sentences. Standard library only.

## Files

`internal/config/config.go` (Config, Database, Load, Parse, Logger), `dotenv.go` (ReadDotEnv),
`config_test.go`, `dotenv_test.go`.

## Exported API

```go
package config

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

type Database struct {
    Driver, URL, Path string // "sqlite"|"postgres"; DATABASE_URL as given or derived; sqlite file path
}

// Load reads ./.env (missing is fine), then the process environment, then overrides
// (highest precedence; used for --mode), and parses the result.
func Load(overrides map[string]string) (Config, error)

// Parse builds a Config from a lookup function (os.LookupEnv shape). Pure; used by tests.
func Parse(lookup func(key string) (string, bool)) (Config, error)

// ReadDotEnv parses a .env file. A missing file returns (nil map, nil).
func ReadDotEnv(path string) (map[string]string, error)

// Logger returns a slog.Logger honouring LogLevel and LogFormat, writing to w.
func (c Config) Logger(w io.Writer) *slog.Logger
```

## Variables, defaults and rules

| Var | Default | Rule |
| --- | --- | --- |
| `MODE` | `all` | one of ingest, worker, api, all |
| `HTTP_ADDR` / `INGEST_ADDR` | `:8080` / `:8081` | non-empty |
| `DATA_DIR` | `/data` | non-empty |
| `DATABASE_URL` | `sqlite://` + `DATA_DIR` + `/feedbaxter.db` (use `path.Join`/`filepath.Join`) | see below |
| `NATS_URL` | empty | required unless MODE=all |
| `KEY_CACHE_RESYNC_INTERVAL` | `5m` | Go duration > 0 |
| `COUNTER_FLUSH_INTERVAL` | `5s` | Go duration > 0 |
| `RETENTION_WINDOW` | `17520h` | Go duration > 0 |
| `RETENTION_CHECK_INTERVAL` | `1h` | Go duration > 0 |
| `DAILY_LIMIT` / `MONTHLY_LIMIT` | `0` / `0` | integer ≥ 0 |
| `RATE_LIMIT_PER_KEY` / `RATE_LIMIT_WINDOW` | `60` / `10s` | integer > 0 / duration > 0 |
| `MAX_BODY_BYTES` / `MAX_MESSAGE_CHARS` / `MAX_METADATA_BYTES` | `16384` / `5000` / `4096` | integers > 0; `MAX_METADATA_BYTES` < `MAX_BODY_BYTES` |
| `QUEUE_MAX_MSGS` / `QUEUE_MAX_BYTES` / `QUEUE_MAX_DELIVER` | `100000` / `268435456` / `5` | integers > 0 |
| `WORKER_CONCURRENCY` | `1` if sqlite, else `4` | integer > 0; the dialect default applies only when unset |
| `WORKER_BATCH_SIZE` / `WORKER_FETCH_MAX_WAIT` | `500` / `250ms` | integer > 0 / duration > 0 |
| `PUBLISH_TIMEOUT` | `2s` | duration > 0 |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` | debug, info, warn, error / text, json (case-insensitive) |

Values are `strings.TrimSpace`d. An empty value counts as unset (default applies). Byte sizes are plain
integers; no unit suffixes. Unknown variables are ignored.

**DATABASE_URL detection**
- `sqlite://<path>` → Driver `sqlite`, Path = everything after `sqlite://`. `sqlite:///data/x.db` gives
  `/data/x.db`; `sqlite://./x.db` gives `./x.db`. Empty path is an error. A `?` in the URL is an error
  ("SQLite pragmas are set by FeedBaxter; remove the query string").
- `postgres://…` or `postgresql://…` → Driver `postgres`, URL passed through unchanged (validated with
  `net/url.Parse` and a non-empty host).
- Anything else → error naming the scheme and listing the two accepted forms. Never echo a password:
  error messages show the scheme only, not the URL.

**Error messages.** Collect every problem and return them with `errors.Join`, one line each, in table
order, shaped `VAR_NAME: <problem>`; e.g.
`RETENTION_WINDOW: "2y" is not a Go duration (examples: 90s, 5m, 17520h)`,
`MODE: "nope" must be one of ingest, worker, api, all`,
`NATS_URL: required when MODE is worker (only MODE=all can embed NATS)`.

## `.env` format (`ReadDotEnv`)

`KEY=VALUE` per line; blank lines and lines whose first non-space char is `#` are skipped; an optional
`export ` prefix is stripped; key and value are trimmed; a value wrapped in matching single or double
quotes has the quotes removed (no escape processing, no `${VAR}` interpolation, no inline comments).
A line without `=` is an error naming the file and line number. Precedence in `Load`:
overrides > process env > `.env` > default.

## Tests (standard library only, table-driven where natural)

`config_test.go` (all via `Parse` with a map-backed lookup unless stated):
- `TestDefaults` – empty env gives every default in the table, Database `{sqlite, sqlite:///data/feedbaxter.db, /data/feedbaxter.db}`, WorkerConcurrency 1.
- `TestEveryVariableOverrides` – one non-default value per variable; each field reflects it.
- `TestDurationsAreGoDurations` – `90s`, `1h30m` accepted; `2y`, `5`, `-1s`, `0s` rejected with the var name and value in the message.
- `TestIntegersValidated` – non-numeric and out-of-range values rejected per the rules.
- `TestAllErrorsReported` – three bad vars → one error containing all three var names.
- `TestModeValidation` – four valid modes; invalid rejected.
- `TestNATSURLRequiredOutsideAll` – worker/api/ingest without NATS_URL fail; all without passes; all with NATS_URL passes.
- `TestDatabaseURLDefaultFollowsDataDir` – `DATA_DIR=/srv/fb` gives Path `/srv/fb/feedbaxter.db`.
- `TestDatabaseURLSQLite` – absolute, relative, empty-path and query-string cases.
- `TestDatabaseURLPostgres` – `postgres://` and `postgresql://` give Driver postgres, URL unchanged.
- `TestDatabaseURLUnknownSchemeHidesPassword` – `mysql://u:secret@h/db` rejected; message has `mysql` and not `secret`.
- `TestWorkerConcurrencyDefaultByDialect` – sqlite → 1, postgres → 4, explicit value wins for both.
- `TestLogLevelAndFormat` – valid and invalid values; `Logger` returns a JSON handler when format is json (assert by writing a record to a buffer and checking it starts with `{`).
- `TestLoadPrecedence` – `t.Chdir(t.TempDir())`, write `.env` with `MODE=worker` and `NATS_URL=nats://x`, `t.Setenv("MODE","api")`, overrides `{"MODE":"ingest"}` → ingest; without the override → api; with neither → worker.

`dotenv_test.go`:
- `TestReadDotEnvSyntax` – comments, blank lines, `export`, both quote styles, `=` inside a value, surrounding spaces.
- `TestReadDotEnvMissingFile` – nil map, nil error.
- `TestReadDotEnvBadLine` – error includes the line number.

## Acceptance

1. `make vet` and `make test` green; package imports only the standard library.
2. Every §11 variable has a field, a default and a test touching it.
3. No duration or limit in this package is used anywhere as a hard-coded constant outside the defaults table.
4. Coder does not touch go.mod, go.sum or any file outside `internal/config/`.
