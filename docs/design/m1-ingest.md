# M1 brief: `internal/ingest` (+ `internal/ingest/keysync`)

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §4, §5, §6, §11; SPEC "Ingest and quota pipeline"; ADR 0003, 0005

The hot path: key cache from a KV watch, per-key token bucket, per-tenant quota counters merged into KV
by CAS, minimal body check, synchronous publish, 202. Plus the DB → KV writer/reconcile used by api mode.

**Hard rule:** `internal/ingest` imports no `internal/store`, `database/sql` or driver. The DB-reading
writer is subpackage `internal/ingest/keysync` (so the no-DB rule is checkable by imports).

**Files.** `ingest/entry.go`, `keycache.go`, `limiter.go`, `counters.go`, `handler.go`, `errors.go`
(error codes/bodies); tests `keycache_test.go`, `limiter_test.go`,
`counters_test.go`, `handler_test.go`, `imports_test.go`. keysync and `ext.KeyWriter` are in `m1-keysync.md`
(same coder). Use `queuetest.Embedded` for NATS.

## Exported API (package ingest)

```go
// Entry is the value of a `keys` KV entry, keyed by HashKey(plaintext). JSON field names are fixed.
type Entry struct {
    KeyID             string `json:"key_id"`
    TenantID          string `json:"tenant_id"`
    ChannelID         string `json:"channel_id"`
    DailyLimit        int64  `json:"daily_limit"`   // 0 = unlimited
    MonthlyLimit      int64  `json:"monthly_limit"` // 0 = unlimited
    Revoked           bool   `json:"revoked"`
    BillingMonthStart int    `json:"billing_month_start"` // 1..28; 0 is read as 1
}
func HashKey(key string) string // lowercase hex sha256; must equal store.HashIngestKey (tested in keysync)

// ---- keycache.go
type KeyCache struct{ /* RWMutex + map[string]Entry, ready chan */ }
func NewKeyCache(log *slog.Logger) *KeyCache
// Run watches kv with WatchAll(ctx, jetstream.IncludeHistory()) until ctx is done. Put → decode and
// store (malformed JSON: warn, remove that key); Delete/Purge → remove. The nil entry that ends the
// initial replay closes Ready (once). If the updates channel closes while ctx is live, re-watch after
// rewatchDelay (named const). Returns nil on ctx cancel; stops the watcher.
func (c *KeyCache) Run(ctx context.Context, kv jetstream.KeyValue) error
func (c *KeyCache) Lookup(hash string) (Entry, bool); func (c *KeyCache) Ready() <-chan struct{}
func (c *KeyCache) Tenants() []TenantWindow // distinct tenants + billing day, for Counters.Load
type TenantWindow struct { TenantID string; BillingMonthStart int }

// ---- limiter.go: token bucket per key ID. Capacity perKey; refill perKey tokens per window, continuous.
func NewLimiter(perKey int, window time.Duration) *Limiter
// Allow takes one token. When empty: false and the wait until one token is available.
func (l *Limiter) Allow(keyID string, now time.Time) (ok bool, retryAfter time.Duration)
// Run sweeps every window, deleting buckets that would be full at now (idle keys). Stops on ctx.
func (l *Limiter) Run(ctx context.Context)

// ---- counters.go
type Result int; const ( Allowed Result = iota; DailyLimited; PlanLimited )
type Verdict struct { Result Result; RetryAfter time.Duration /* DailyLimited only */; dayKey, monthKey string }
func NewCounters(js jetstream.JetStream, kv jetstream.KeyValue, flushEvery time.Duration, log *slog.Logger) *Counters
// Load reads the current day and month keys for each tenant from KV before serving.
func (c *Counters) Load(ctx context.Context, tenants []TenantWindow, now time.Time) error
// TryIncr: under the tenant's mutex, if Monthly>0 and shared+delta+1 > Monthly → PlanLimited; else if
// Daily>0 and day shared+delta+1 > Daily → DailyLimited (RetryAfter = time to next UTC midnight);
// else increment both local deltas → Allowed. PlanLimited wins when both are exceeded (it means
// "stop retrying"; daily would make SDKs retry at midnight into a still-full month).
func (c *Counters) TryIncr(tenantID string, lim ext.Limits, billingDay int, now time.Time) Verdict
// Refund undoes an Allowed verdict's increments (used when the request then fails: 400 or 503),
// so only accepted (202) submissions count.
func (c *Counters) Refund(v Verdict)
// Flush: for every tracked key: Get → (KeyNotFound = value 0, rev 0) → if delta != 0,
// queue.PutTTL(value+delta, rev, ttl) where ttl = queue.CounterDayTTL / CounterMonthTTL; on
// queue.IsWrongLastSequence re-Get and retry up to maxFlushAttempts (named const), then leave the delta
// for the next flush and log a warning. On success shared = written value, delta -= flushed amount
// (deltas added during the round trip are kept). With delta 0, just refresh shared (sees other
// replicas). Drops keys of past windows whose delta is 0. Returns the first error (still tries all keys).
func (c *Counters) Flush(ctx context.Context) error
// Run flushes every flushEvery until ctx is done, then does one final Flush on a context detached
// from ctx (context.WithoutCancel + finalFlushTimeout const).
func (c *Counters) Run(ctx context.Context)
func (c *Counters) Snapshot() map[string][2]int64 // tenant → local (day, month) totals: shared+delta
```
Key naming (§5): `<tenant>.day.<YYYY-MM-DD>` (UTC date) and `<tenant>.month.<YYYY-MM-DD>` where the
date is the billing period start: day `b` (1..28, 0→1) of this UTC month if today's day ≥ b, else day b of
the previous month. Value: decimal int64 text.

## Handler

```go
type Config struct { MaxBodyBytes int64; MaxMessageChars int; MaxMetadataBytes int64; PublishTimeout time.Duration }; func ConfigFrom(config.Config) Config
type Deps struct {
    Cache *KeyCache; Limiter *Limiter; Counters *Counters
    JS jetstream.JetStream; Config Config; Log *slog.Logger
    Now func() time.Time // nil = time.Now
}
func NewHandler(d Deps) http.Handler // the caller runs Cache.Run, Limiter.Run, Counters.Run
```
Order, exactly (§4): every response carries `Access-Control-Allow-Origin: *` and
`Access-Control-Expose-Headers: Retry-After` (browser SDKs must read it).
1. `OPTIONS` → also `-Allow-Methods: POST, OPTIONS`, `-Allow-Headers: Authorization, Content-Type`,
   `-Max-Age: 86400`; 204. No key lookup. Other non-POST → 405 `method_not_allowed`.
2. If `Cache.Ready()` is not closed → 503 `queue_full`, Retry-After 5 (an unready cache must not 401
   good keys). Bearer token (case-insensitive scheme) → HashKey → Lookup; miss/revoked/no header → 401 `invalid_key`.
3. `io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))`; `*http.MaxBytesError` → 413 `payload_too_large`.
4. `Limiter.Allow(entry.KeyID, now)` → 429 `rate_limited`, Retry-After = ceil seconds, min 1.
5. `Counters.TryIncr(entry.TenantID, {Daily, Monthly from entry}, entry.BillingMonthStart, now)` →
   429 `daily_limit` (Retry-After = ceil seconds to next UTC midnight) or 429 `plan_limit` (no Retry-After).
6. Body check into a struct of `json.RawMessage` fields: top level is a JSON object; `message` a string,
   `strings.TrimSpace` non-empty, `utf8.RuneCountInString ≤ MaxMessageChars`; `metadata` absent/null
   or an object with raw length ≤ MaxMetadataBytes; `idempotencyKey` absent or a non-empty string ≤ 128
   runes; `userId`, `appVersion`, `platform` absent/null or strings. Else 400 `invalid_payload` + Refund.
   Unknown fields are ignored. The body is not modified.
7. Headers: FB-Tenant, FB-Channel, FB-Key (key ID), FB-Received (`now.UTC().Format(time.RFC3339Nano)`),
   FB-Idem (body's idempotencyKey, or a generated 26-char lowercase base32 crypto/rand value),
   `Nats-Msg-Id` = tenant + ":" + idem. `js.PublishMsg` with a `PublishTimeout` context — synchronous,
   never PublishAsync. Any error → Refund + 503 `queue_full`, Retry-After 5 (`queue.IsQueueFull`
   logs at debug; other errors at warn). A duplicate PubAck is still 202.
8. 202, empty body.
Error body every time: `{"error":"<code>","message":"<plain sentence>"}`, `Content-Type: application/json`.
Never read/log client IP or key plaintext. Request logging at debug only.

## Tests (handler tests: httptest + queuetest.Embedded + a KeyCache fed by Run on the real `keys` KV)

handler_test.go: `TestPreflight` (204, the four CORS headers, works with no Authorization);
`TestInvalidKeyUnknown`, `TestInvalidKeyRevoked`, `TestInvalidKeyMissingHeader` (401 + body shape);
`TestPayloadTooLarge` (413); `TestInvalidPayload` table: not JSON, array, message missing / not string /
empty / whitespace / MaxMessageChars+1 runes (multi-byte runes at exactly the limit pass), metadata
array / too large, idempotencyKey 129 chars, userId number → all 400 and the tenant counter is unchanged;
`TestRateLimited` (perKey 2: third → 429, Retry-After ≥ 1); `TestDailyLimit` (Daily 1, Now fixed at
23:59:30 UTC → second request 429 daily_limit, Retry-After "30"); `TestPlanLimit` (Monthly 1 → 429
plan_limit, no Retry-After); `TestQueueFull` (stream MaxMsgs 1: second → 503 queue_full, Retry-After 5,
counters refunded); `TestAcceptedPublishesExactMessage` (202 empty body; fetch from `processor`: data is
byte-identical to the request body, all six headers exact, FB-Received parses as RFC3339Nano);
`TestIdempotencyKeyGenerated` (no key in body → FB-Idem 26 chars, Nats-Msg-Id = tenant:idem);
`TestNotReady` (cache not ready → 503); `TestDuplicateStillAccepted` (same idempotencyKey twice → both
202, stream holds 1).
keycache_test.go: `TestKeyCacheReplayThenReady` (entries put before Run are present when Ready closes);
`TestKeyCacheLiveUpdates` (create, revoke, limit change, delete each visible via Lookup — wait by polling
Lookup with a 5s deadline, no fixed sleeps); `TestKeyCacheEmptyBucketReady`; `TestKeyCacheMalformed`.
limiter_test.go: `TestLimiterBucket` (injected times: burst perKey, refill after window/perKey, retryAfter
exact); `TestLimiterPerKeyIsolation`; `TestLimiterSweep`.
counters_test.go: `TestCountersKeyNames` (billing day 1/15/28, month boundary, 0→1); `TestCountersDaily`
/ `TestCountersMonthly` / `TestCountersPlanWins`; `TestCountersFlushAndLoad` (flush, new instance Loads
the value, TTL header present on the KV message); `TestCountersTwoReplicasConverge` (two Counters on one
KV, interleaved increments and Flushes incl. a forced conflict → final KV value == total accepted, both
Snapshots equal after a refresh flush); `TestCountersOvershootBound` (limit L, A and B each accept until
rejected between flushes: total accepted ≤ L + accepted-since-last-flush per replica, and after both flush
each rejects the very next request); `TestCountersRefund`; `TestCountersRunFinalFlush` (cancel ctx →
Run returns and KV holds the delta).
imports_test.go: `TestIngestHasNoDatabaseImports` (go/parser over non-test files of the package: no
`internal/store`, `database/sql`, pgx or sqlite import).

## Acceptance

`make vet`, `make test` green. No DB import in `internal/ingest`; no
PublishAsync; no literal durations outside named consts; every goroutine stops on ctx; the CAS loop
never loses a delta (conflict → retry; give-up → kept for next flush).
