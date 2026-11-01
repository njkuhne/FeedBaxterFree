# FeedBaxter architecture

Engineering source of truth. The product source of truth is `docs/SPEC.md`; reasons for the choices here are in `docs/adr/`. Component design briefs derived from this document live in `docs/design/`.

## 1. Shape

One Go module, one binary, four modes.

```
                 ┌──────────── ingest (stateless) ────────────┐
POST /v1/feedback│ sha256(key) → key cache (map, from KV watch)│
   + CORS        │ MaxBytesReader → minimal JSON check         │  sync publish, ack
                 │ token bucket per key_id (memory)            │─────────────► JetStream stream FEEDBACK
                 │ daily/monthly atomics per tenant (memory)   │               subject feedback.ingest
                 │   └─ flush: CAS merge into KV `counters`    │               MaxMsgs/MaxBytes, DiscardNew
                 └─────────────────────────────────────────────┘                        │ pull consumer, batch
                                                                                        ▼
                 ┌──────────── worker (stateless, ×N) ─────────┐              ┌─────────────────────┐
                 │ Fetch(500) → validate/normalise → one tx    │              │ SQLite (self-host)  │
                 │ INSERT … ON CONFLICT DO NOTHING → ack       │─────────────►│ Postgres (SaaS)     │
                 │ final delivery → publish feedback.dlq, ack  │              │ same migrations     │
                 │ retention ticker                            │              └──────────▲──────────┘
                 └─────────────────────────────────────────────┘                         │
                 ┌──────────── api ────────────────────────────┐                         │
                 │ JSON API + sessions + go:embed React UI      │─────────────────────────┘
                 │ MCP (streamable HTTP, PAT)                   │
                 │ writes KV `keys` on key/limit change         │────► KV `keys` ──watch──► ingest cache
                 │ DB→KV reconcile every KEY_CACHE_RESYNC_INTERVAL
                 │ ops endpoint: queue depth, oldest age, DLQ   │
                 └──────────────────────────────────────────────┘
`all` mode = the three above in one process + embedded nats-server (in-process connection, no TCP listener).
```

| Mode | Runs | Needs |
| --- | --- | --- |
| `ingest` | HTTP ingest server, key cache, limiter, counters, publisher | NATS URL |
| `worker` | Pull consumer, batch inserts, DLQ, retention ticker | NATS URL, DB |
| `api` | JSON API, sessions, embedded UI, MCP, KV writer and reconcile, ops | NATS URL, DB |
| `all` | All three plus embedded nats-server with JetStream | DB path, data dir |

Self-host uses `all` with SQLite. SaaS runs each mode as its own deployment against Postgres and a standalone NATS cluster.

## 2. Module and package layout

Module path: `github.com/njkuhne/feedbaxterfree` (repo: https://github.com/njkuhne/FeedBaxterFree).

```
cmd/feedbaxter/        main: `serve --mode=<ingest|worker|api|all>`, `migrate`, `reset-admin`
feedbaxter.go          package feedbaxter: Run(ctx, Options) and Options. The only wiring entry point.
ext/                   exported extension surface (interfaces and Deps) used by cmd/ and by the private SaaS repo
internal/config        env → typed Config; Go durations; validation; defaults
internal/queue         embedded server start, connection, stream/consumer/KV bucket declarations, names
internal/ingest        http.Handler, keycache, limiter, counters, publisher, error responses, CORS
internal/processor     consumer loop, batch insert, DLQ, retention job
internal/store         Open, Dialect, migrations (embedded SQL), query functions by entity
internal/api           router, session auth, handlers, export, ops endpoint, embedded UI serving
internal/mcp           MCP server and tools
internal/ops           health and readiness, in-process metrics snapshot
web/                   React + Vite + TypeScript admin UI; `web/dist` is embedded by internal/api
sdk/web/               npm package (@feedbaxter/web, MIT) and script tag build; see sdk/web/README.md
sdk/swift/             Swift package
deploy/                Dockerfile (scratch, multi-stage), docker-compose.yml (self-host), compose.dev.yml (Postgres + NATS for tests)
docs/                  SPEC.md, REVIEW.md, ARCHITECTURE.md, design/, adr/
Makefile               every build, test and image target; all Go runs inside the golang Docker image
```

Rule: `internal/` packages never import `ext/` implementations, only interfaces. `feedbaxter.go` is the only place that knows about every package.

## 3. Extension surface (`ext/`)

The private SaaS repo imports this module and fills `feedbaxter.Options`. Everything it needs must be reachable from `ext/`; nothing in `internal/` is.

```go
package ext

type Tenant struct { ID, Name string; BillingMonthStart int /* day of month */ }

// TenantResolver maps an authenticated principal or an ingest key to a tenant.
// Self-host: always the single tenant. SaaS: by row.
type TenantResolver interface {
    ByID(ctx context.Context, id string) (Tenant, error)
    // Default is the tenant used when there is exactly one (self-host bootstrap, CLI).
    Default(ctx context.Context) (Tenant, error)
}

// Authenticator produces the session principal for the api mode.
// Self-host: password + sessions, one-time admin bootstrap. SaaS: email flows (private).
type Authenticator interface {
    Mount(mux *http.ServeMux, deps Deps)           // login, logout, password change, invite flows
    Principal(r *http.Request) (Principal, bool)   // resolves the session cookie
}

type Principal struct { UserID, TenantID string; Role Role; Via string /* "ui" | "mcp" */ }

// LimitsSource decides daily and monthly limits for a tenant. Self-host: config. SaaS: plans.
type LimitsSource interface {
    Limits(ctx context.Context, tenantID string) (Limits, error)
}
type Limits struct { Daily, Monthly int64 } // 0 = unlimited

// Deps is what extensions get: the DB handle and dialect, the KV writer for key-cache entries,
// the session store, the logger, and Settings (a small exported view of config). Exported types only.
type Deps struct { ... }

type Options struct {
    Mode            string
    TenantResolver  TenantResolver
    Authenticator   Authenticator
    LimitsSource    LimitsSource
    Migrations      []Migration        // extra migrations, applied after core, own version namespace
    MountRoutes     func(mux *http.ServeMux, deps Deps)
    OnStart         []func(ctx context.Context, deps Deps) error
}
```

Exact signatures are fixed in the M1/M2 design briefs; the shape above is binding.

## 4. Ingest hot path

Order of operations in `internal/ingest`, in one handler, no allocations beyond the body buffer where avoidable:

1. `OPTIONS` → CORS headers (`Access-Control-Allow-Origin: *`, `-Headers: Authorization, Content-Type`, `-Max-Age: 86400`), `204`. No key lookup.
2. `Authorization: Bearer <key>` → `sha256(key)` → `keycache.Lookup(hash)`. Miss or `revoked` → `401 invalid_key`.
3. `http.MaxBytesReader(w, r.Body, MaxBodyBytes)` (default 16 KiB). Overflow → `413 payload_too_large`.
4. Rate limit: token bucket per key ID, `RATE_LIMIT_PER_KEY` tokens per `RATE_LIMIT_WINDOW`. Reject → `429 rate_limited`, `Retry-After`.
5. Counters: `counters.TryIncr(tenantID, limits)`; daily over → `429 daily_limit` with `Retry-After` = seconds to next UTC midnight; monthly over → `429 plan_limit`. Increment is atomic and checked against the shared total plus local delta.
6. Minimal body check: valid JSON object; `message` present, non-empty, ≤ `MaxMessageChars` (default 5000); `metadata` if present is an object and its raw bytes ≤ `MaxMetadataBytes` (default 4 KiB); `idempotencyKey` if present ≤ 128 chars. Anything else → `400 invalid_payload`. No normalisation here.
7. Publish: `js.Publish(subject, body, headers)` synchronous with `PublishTimeout` (default `2s`). Headers: `FB-Tenant`, `FB-Channel`, `FB-Key`, `FB-Received` (RFC 3339 nano, UTC), `FB-Idem`, and `Nats-Msg-Id = tenant + ":" + idem`. The stream's `DiscardNew` rejection → `503 queue_full` with `Retry-After: 5`. Timeout → `503 queue_full`.
8. `202 Accepted`, empty body.

Error body shape, every error: `{"error":"<code>","message":"<plain sentence>"}`.

Readiness: `/readyz` is `503` until the key-cache replay from KV has completed; `/healthz` is `200` whenever the process serves.

## 5. Key cache and counters

**KV bucket `keys`.** Key: hex `sha256(ingest key)`. Value (JSON): `{"key_id","tenant_id","channel_id","daily_limit","monthly_limit","revoked":bool}`. Written by `internal/api` on key create, key revoke, channel delete, tenant delete, and on any `LimitsSource` change (SaaS plan change, limit edit). Deleting a tenant purges its keys. The api mode also runs the reconcile: every `KEY_CACHE_RESYNC_INTERVAL`, load all keys from the DB and upsert entries whose value differs; delete entries with no row. Ingest watches with `IncludeHistory` so startup replay is the bootstrap.

**KV bucket `counters`.** Keys: `<tenant>.day.<YYYY-MM-DD>` and `<tenant>.month.<YYYY-MM-DD of billing period start>`. Value: decimal int64. Each ingest replica keeps `local delta` per key and, every `COUNTER_FLUSH_INTERVAL`, does `get → add delta → Update(rev)`; on revision conflict, re-get and retry. On startup, the replica reads current values before serving. Entries carry a TTL of 40 days (daily) and 400 days (monthly) via the bucket's per-key TTL. Self-host with no limits still counts, so the UI can show usage.

The "very next request" rejection guarantee is per replica; cross-replica overshoot is bounded by one flush interval.

## 6. Queue

| Object | Name | Config |
| --- | --- | --- |
| Stream | `FEEDBACK` | subjects `feedback.ingest`; `MaxMsgs` = `QUEUE_MAX_MSGS` (default 100000), `MaxBytes` = `QUEUE_MAX_BYTES` (default 256 MiB); `Discard: New`; `Retention: WorkQueue` (an acked message leaves the stream; with the default limits policy acked messages would fill a DiscardNew stream and ingest would 503 forever); file storage; `Duplicates` window 2 min |
| Consumer | `processor` | durable, pull, `AckExplicit`, `MaxDeliver` unlimited (-1) so JetStream never strands a message; `AckWait` 30 s, `MaxAckPending` 2000. `QUEUE_MAX_DELIVER` is applied by the processor, not the consumer (§7) |
| Stream | `FEEDBACK_DLQ` | subject `feedback.dlq`; `MaxMsgs` 10000; `Discard: Old` (DLQ may roll, main queue never does) |
| KV | `keys` | history 1 |
| KV | `counters` | history 1, per-key TTL enabled |

`internal/queue.Ensure(ctx, js)` declares all of them idempotently at startup of every mode. Embedded server in `all` mode: `server.Options{JetStream: true, StoreDir: DATA_DIR/nats, DontListen: true}`, connected with `nats.InProcessServer`.

## 7. Processor

Loop per worker goroutine (`WORKER_CONCURRENCY`, default 1 for SQLite, 4 for Postgres):

1. `cons.Fetch(BatchSize, FetchMaxWait)` (defaults 500, 250 ms).
2. For each message: parse headers and body; normalise (`strings.TrimSpace` message, clamp lengths, `platform` lower-cased, `metadata` re-encoded compact). Unparseable → publish to `feedback.dlq` with the original headers plus `FB-Error`, then `Term`.
3. One transaction: `INSERT INTO feedback … ON CONFLICT (tenant_id, idempotency_key) DO NOTHING` per row via a prepared statement (multi-row VALUES on Postgres). Commit.
4. Ack every message in the batch. On error, two failure classes:
   - **Transient** (database unreachable: begin or commit failure, `sql.ErrConnDone`, `driver.ErrBadConn`, context errors, any `net.Error`, or any insert error while a ping also fails): never dead-letter. `NakWithDelay(min(1 s × NumDelivered, 30 s))` and pause fetching for that long. Retries are unbounded; the consumer has no `MaxDeliver`.
   - **Rejected row** (database reachable, the row itself fails): retry each row of the batch in its own transaction so one poison row does not drag the batch down; a row with `NumDelivered >= QUEUE_MAX_DELIVER` is dead-lettered and acked, otherwise `Nak` with the same backoff but without pausing the worker.
   - A failed DLQ publish is infrastructure trouble: `Nak` with backoff and pause; never ack or term.

Retention: a jittered ticker (`RETENTION_CHECK_INTERVAL`, default `1h`) in every worker deletes rows older than `RETENTION_WINDOW` (default `17520h`, 2 years) in batches of 1000 per transaction until none remain.

## 8. Store

- `store.Open(cfg)` returns `*store.DB{SQL *sql.DB, Dialect}`; SQLite driver `modernc.org/sqlite` with `_pragma=journal_mode(WAL)`, `synchronous(NORMAL)`, `busy_timeout(5000)`, `foreign_keys(ON)`; Postgres via `pgx/v5/stdlib`.
- Migrations: embedded `.sql` files `NNNN_name.sqlite.sql` and `NNNN_name.postgres.sql` only where the dialects differ; otherwise a single `NNNN_name.sql` in which the token `{{json}}` expands to the dialect's JSON column type. Each migration runs in its own transaction with its `schema_migrations` row; on Postgres an advisory lock serialises replicas that start together. A `schema_migrations` table records versions. Extensions append their own files in a separate version namespace (`ext_NNNN_…`).
- Dialect shim, and nothing more: placeholder rewrite (`?` → `$n`), `JSON` column type (`TEXT` vs `JSONB`), `NOW()` vs `CURRENT_TIMESTAMP`, `INSERT … RETURNING` is supported by both.
- Queries are hand-written functions per entity in `internal/store/<entity>.go`, taking `context.Context` and a `Querier` (so they run in or out of a transaction).
- IDs: 20-char random base32 strings generated in Go with a short type prefix (`tn_`, `us_`, `ch_`, `ky_`, `tk_`, `fb_`, `tg_`, `sc_` for status changes), `TEXT PRIMARY KEY` in both databases. No autoincrement, no UUID type dependency.
- Timestamps: `BIGINT` Unix milliseconds in both databases (Postgres `INTEGER` is 32-bit; SQLite treats `BIGINT` as integer affinity), converted in Go. One representation, no timezone bugs. Booleans are `INTEGER` 0/1.
- The `plans` table is SaaS-only and lives in an extension migration (`ext_NNNN_…`); `tenants.plan_id` is a nullable `TEXT` with no foreign key.
- `feedback.channel_id` has no foreign key: a batch must never fail because a channel was deleted between ingest and insert. What channel deletion does to existing feedback is decided in M2.

Schema is the spec's data model table. Indexes: `(tenant_id, status, received_at DESC)`, `(tenant_id, channel_id, received_at DESC)`, `(tenant_id, user_id)`, unique `(tenant_id, idempotency_key)`, unique `(tenant_id, name)` on tags and on channels.

## 9. API mode

- `net/http` with Go 1.22+ patterns. JSON in and out. Every handler gets a `Principal` from the `Authenticator` or returns `401`.
- Sessions: `sessions` table (id, user_id, expires_at, created_at); cookie `fb_session`, `HttpOnly`, `Secure` when the request is HTTPS, `SameSite=Lax`, 30-day expiry sliding. Mutating requests require `Sec-Fetch-Site` of `same-origin`/`none` or an `X-Requested-With` header; otherwise `403`.
- Passwords: argon2id (`golang.org/x/crypto/argon2`), OWASP profile t=2, m=19 MiB, p=1, 16-byte salt, encoded PHC string; parameters are read back from the string, so hashes from the earlier 64 MiB profile still verify. At most two hashes run at once and the working set is returned to the OS after each one (`debug.FreeOSMemory`), otherwise one hash raises the GC heap goal above 128 MB and the process keeps that memory for minutes.
- Business rules live in `internal/service`; every method that takes a principal starts with `requireUser` or `requireAdmin`, and `requireAdmin` is the only role check. `internal/api` only maps HTTP to service calls and errors, so the MCP server reuses the same checks.
- Export: streams CSV or JSON from a cursor, honours the inbox filter parameters.
- Ops endpoint `/api/ops`: queue depth, oldest message age, DLQ count, counter snapshot, DB size (admin only).
- Static UI: `go:embed web/dist`; SPA fallback to `index.html`; immutable cache headers on hashed assets; fonts served from the same embed.

## 10. MCP

`internal/mcp` uses `github.com/modelcontextprotocol/go-sdk`, Streamable HTTP mounted at `/mcp`. Auth middleware: `Authorization: Bearer <PAT>` → `personal_access_tokens` row by hash → `Principal{Via: "mcp"}`. Tools are thin wrappers over `internal/service`, so permission checks and history (`via: mcp`) are shared, not duplicated. Bulk calls accept up to 500 IDs. The transport is stateless with JSON responses (no sessions, no streaming). The SDK's localhost DNS-rebinding guard is turned off because every request needs a bearer token and the guard breaks reverse proxies on the same host. The handler is mounted at `/mcp` and `/mcp/` so every sub-path answers 404.

## 11. Config

All from env (a `.env` file is read if present, env wins). Durations are Go durations.

| Var | Default | Mode |
| --- | --- | --- |
| `MODE` | `all` | all |
| `HTTP_ADDR` | `:8080` | api, all (UI + API + MCP) |
| `INGEST_ADDR` | `:8081` | ingest; in `all` the ingest handler is also mounted on `HTTP_ADDR` at `/v1/feedback` |
| `DATA_DIR` | `/data` | all (SQLite file and NATS store) |
| `DATABASE_URL` | `sqlite://$DATA_DIR/feedbaxter.db` | worker, api, all; `postgres://…` selects Postgres |
| `NATS_URL` | empty = embedded | ingest, worker, api (required unless `all`) |
| `KEY_CACHE_RESYNC_INTERVAL` | `5m` | api |
| `COUNTER_FLUSH_INTERVAL` | `5s` | ingest |
| `RETENTION_WINDOW` | `17520h` | worker |
| `RETENTION_CHECK_INTERVAL` | `1h` | worker |
| `DAILY_LIMIT` / `MONTHLY_LIMIT` | `0` (unlimited) | self-host `LimitsSource` |
| `RATE_LIMIT_PER_KEY` / `RATE_LIMIT_WINDOW` | `60` / `10s` | ingest |
| `MAX_BODY_BYTES` / `MAX_MESSAGE_CHARS` / `MAX_METADATA_BYTES` | `16384` / `5000` / `4096` | ingest, worker |
| `QUEUE_MAX_MSGS` / `QUEUE_MAX_BYTES` | `100000` / `268435456` | queue |
| `QUEUE_MAX_DELIVER` | `5` | worker: attempts at inserting a row the database rejects before it is dead-lettered; outages retry forever |
| `WORKER_CONCURRENCY` / `WORKER_BATCH_SIZE` / `WORKER_FETCH_MAX_WAIT` | `1` (SQLite) or `4` / `500` / `250ms` | worker |
| `PUBLISH_TIMEOUT` | `2s` | ingest |
| `LOG_LEVEL` / `LOG_FORMAT` | `info` / `text` (`json` for SaaS) | all |

Logging: `log/slog`. Request logs at debug only on ingest (it is the hot path).

## 12. Dependencies

Go 1.27. Allowed direct dependencies in the public module, and the only ones:

| Module | For |
| --- | --- |
| `github.com/nats-io/nats-server/v2` | embedded server (`all` mode) |
| `github.com/nats-io/nats.go` | client, JetStream, KV |
| `modernc.org/sqlite` | SQLite, pure Go |
| `github.com/jackc/pgx/v5` | Postgres |
| `golang.org/x/crypto` | argon2id |
| `github.com/modelcontextprotocol/go-sdk` | MCP server |

Anything else needs an ADR. Test helpers use the standard library only.

## 13. Toolchain and testing

- No Go on the dev machine. `make` targets run `docker run --rm -v $(PWD):/src -v fbx-gomodcache:/go/pkg/mod -w /src golang:1.27-alpine …`. Targets: `build`, `test`, `test-pg`, `lint` (`go vet`), `image`, `size-check`, `rss-check`, `ui`, `dev`.
- `make test` runs `go test ./...` with SQLite in-process and an embedded NATS per test package (fast, no containers). `make test-pg` starts Postgres from `deploy/compose.dev.yml` and runs the store and API tests again with `TEST_DATABASE_URL`.
- Every store and API test is written once and parameterised over both dialects via a `storetest.ForEachDialect(t, func(t *testing.T, db *store.DB))` helper that skips Postgres when `TEST_DATABASE_URL` is unset.
- `make image` builds `deploy/Dockerfile` (multi-stage: node for `web/`, golang for the binary, `scratch` for the final image with `ca-certificates` and a non-root user). `make size-check` fails if the binary exceeds 32 MiB; `make rss-check` boots the image, signs in once and fails if the process RSS exceeds 48 MiB. CI runs both.
- UI: Vite, React, TypeScript, no component library, no CSS framework; design tokens in one CSS file from the Styleguide. Fonts subset at build time into `web/public/fonts`.

## 14. Milestones

See `docs/REVIEW.md` section 5. Acceptance for each milestone is run by Fable from that table; Opus's briefs in `docs/design/` carry the per-component tests that add up to it.
