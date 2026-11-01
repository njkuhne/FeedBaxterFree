# M1 brief: wiring (`feedbaxter.go`, `internal/selfhost`, `ext.Deps`, `make dev`)

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §1, §3, §4 (readiness), §11; SPEC "Data model"

Make `feedbaxter serve` run the pipeline. Done when, in `all` mode on SQLite, first boot logs an ingest
key once and `curl -X POST …/v1/feedback -H 'Authorization: Bearer <key>' -d '{"message":"hi"}'` returns
202 and a row appears in `feedback`.

**Files.** `feedbaxter.go` (Run, modes, shutdown), `serve.go` (new, root package: HTTP mux, health,
component lifecycle helpers), `internal/selfhost/selfhost.go` + `selfhost_test.go`, `ext/ext.go` (Deps
gains `Keys KeyWriter`), `internal/ingest/handler.go` (Deps gains `Ready <-chan struct{}`, nil = Cache.Ready()),
`feedbaxter_test.go`, `Makefile` (`dev`), `.env.example` only if a comment is wrong. go.mod frozen.

## internal/selfhost

```go
package selfhost
const TenantName = "Self-host"; const DefaultChannel = "Default"
// TenantResolver is the single-tenant ext.TenantResolver: Default = store.FirstTenant, ByID = store.GetTenant.
type TenantResolver struct{ DB *store.DB }
// Limits is the self-host ext.LimitsSource: the same DAILY_LIMIT / MONTHLY_LIMIT for every tenant.
type Limits struct{ Daily, Monthly int64 }
func LimitsFromConfig(cfg config.Config) Limits
// Bootstrap: in one transaction, if CountTenants == 0 create tenant TenantName (billing day 1), channel
// DefaultChannel and one ingest key (store.GenerateIngestKey). After commit: keys.SyncKey(keyID) (an error
// is logged, not returned — the reconcile repairs it), then log once at info exactly
// `ingest key for channel Default: fbx_…` (the key in the message text so it is greppable in text and
// json logs). Returns created=false and logs nothing when a tenant exists.
func Bootstrap(ctx context.Context, db *store.DB, keys ext.KeyWriter, log *slog.Logger, now time.Time) (created bool, err error)
```
Admin user/password is M2. `ext.Deps` gets `Keys ext.KeyWriter` (doc comment). The NATS connection is
NOT added to Deps in M1: ext must stay standard-library-only and nothing consumes it yet (M2 decides).

## Run

`config.Load` (MODE override as now) → logger from config writing to unexported package var
`logOutput io.Writer = os.Stderr` (tests capture it). Defaults: nil `TenantResolver` → selfhost resolver
**and** Bootstrap runs; non-nil → no Bootstrap (SaaS creates tenants itself). Nil `LimitsSource` →
`selfhost.LimitsFromConfig`. Queue: `queue.Start` when `NATSURL == ""` (only valid in all), else
`queue.Connect`; then `queue.Ensure(ctx, js, queue.LimitsFromConfig(cfg))` in every mode.

| Mode | Starts | HTTP |
| --- | --- | --- |
| all | store.Open + Migrate(opts.Migrations); queue; Ensure; Bootstrap (selfhost); keysync.Writer.Run(KEY_CACHE_RESYNC_INTERVAL); ingest stack; processor | HTTP_ADDR: `/v1/feedback`, `/healthz`, `/readyz`, Authenticator.Mount + MountRoutes when set |
| ingest | queue (Connect); Ensure; ingest stack. **Run never opens a DB in ingest mode.** | INGEST_ADDR: `/v1/feedback`, `/healthz`, `/readyz` |
| worker | store.Open + Migrate; queue; Ensure; processor | HTTP_ADDR: `/healthz`, `/readyz` |
| api | store.Open + Migrate; queue; Ensure; Bootstrap (selfhost); keysync Run | HTTP_ADDR: `/healthz`, `/readyz`, Mount + MountRoutes |

Ingest stack: KeyCache.Run(objects.Keys); Limiter.Run; once Cache.Ready() is closed, `Counters.Load(ctx,
cache.Tenants(), time.Now())`, then close the process `ready` channel and start Counters.Run. The ingest
handler gets `Ready: ready` so it answers 503 until counters are loaded too. `/readyz`: 200 once `ready`
is closed (worker/api: once startup finished), else 503 with the standard error body shape
`{"error":"not_ready",…}`. `/healthz`: always 200 `ok`. Mount `/v1/feedback` as a plain path (the handler
does its own method checks; OPTIONS must reach it). OnStart hooks run after startup with `ext.Deps{DB,
Dialect, Logger, Keys}` (worker/ingest: DB nil where not opened, Keys nil in ingest/worker); a hook error
aborts Run with cleanup.

`http.Server` with `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout` as named consts.
Listen with `net.Listen` first so a bind error is returned from Run immediately (after cleanup).

## Shutdown (when ctx is done, or any component fails)

1. `srv.Shutdown` with `shutdownTimeout` const (stop accepting; in-flight requests finish, so every
   accepted 202 was already published synchronously).
2. Cancel the components' context; wait for all goroutines (errgroup-style with sync.WaitGroup; no new
   dep): Processor.Run finishes and acks its in-flight batch; Counters.Run does its final flush;
   KeyCache, Limiter, keysync stop.
3. `queue.Conn.Close()` (drains, stops embedded server), then `db.Close()`.
Run returns nil on a clean ctx-cancel shutdown; the first component/listen error otherwise. A fatal
component error (e.g. processor returns error, listener dies) triggers the same sequence.

## make dev

```make
DEV_PORT ?= 8090
dev: ## Run serve (all mode, SQLite in ./data) on localhost:$(DEV_PORT); 8090 because 8080 is taken on this machine
	mkdir -p data
	$(DOCKER_RUN_DEV) go run ./cmd/feedbaxter serve
```
Define `DOCKER_RUN_DEV` like `DOCKER_RUN` plus `--init -p $(DEV_PORT):8080 -e DATA_DIR=/src/data
-e HTTP_ADDR=:8080` (`--init` so Ctrl-C reaches the process; no `-it` requirement). Remove the M0 comment
that says `dev` arrives in M1. `/data/` is already gitignored.

## Tests

`internal/selfhost/selfhost_test.go` (ForEachDialect + queuetest + a real keysync.Writer):
`TestBootstrapCreatesOnce` (tenant/channel/key rows; KV entry under store.HashIngestKey(logged key);
log line matches `ingest key for channel Default: fbx_[a-z2-7]{32}`; second call → created=false, no
log output, still one tenant); `TestTenantResolver`; `TestLimitsFromConfig`.

`feedbaxter_test.go` (replace `TestRunNotImplemented`; keep the rest):
- `TestRunAllEndToEnd` – isolate (temp dir, chdir, DATA_DIR + DATABASE_URL into it), HTTP_ADDR on a
  free 127.0.0.1 port (net.Listen ":0", read port, close), capture `logOutput`; Run in a goroutine; poll
  `/readyz` until 200 (deadline 10s); `/healthz` 200; take the key from the log; POST
  `{"message":"hi"}` → 202; OPTIONS preflight → 204; poll a second `store.Open` of the same SQLite file
  until `SELECT message FROM feedback` returns "hi" (deadline 10s); bad key → 401. Cancel; Run returns
  nil within 5s.
- `TestRunAllRestartKeepsKey` – Run, read key, stop; Run again on the same dir: no key logged the second
  time; the first key still gets 202 (KV and DB persisted; bootstrap ran once).
- `TestRunBindError` – HTTP_ADDR already held by a listener → Run returns an error quickly, no hang.
- `TestRunIngestModeNeedsNATS` – mode ingest without NATS_URL → config error (exists in config; assert
  via Run).
`cmd/feedbaxter/main_test.go`: drop or adapt anything that asserted serve is unimplemented.

## Acceptance

`make vet`, `make test`, `make test-pg` green; paste output. `make dev` + the curl round trip works
(paste the log line with the key redacted after 12 chars, the curl status, and a `sqlite3`-free proof
the row exists, e.g. a second curl to /readyz plus the test). No goroutine outlives Run; Run returns
within a few seconds of cancel. Ingest mode never opens a DB.
