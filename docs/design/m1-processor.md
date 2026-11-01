# M1 brief: `internal/processor`

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §6, §7, §8, §11; SPEC "Processors", "Privacy, retention"

Pull batches from the `processor` consumer, normalise, insert in one transaction, ack. Dead-letter what
cannot be parsed or cannot be stored after `MaxDeliver` attempts (database outages retry forever). Delete rows past the retention window.
Processors know nothing about quotas, keys or billing.

**Files.** `internal/processor/processor.go` (Config, New, Run, batch loop), `normalise.go`,
`retention.go`; tests `normalise_test.go`, `processor_test.go`, `retention_test.go`. Uses
`store.InsertFeedbackBatch`, `store.DeleteFeedbackOlderThan`, `queue` names/headers, `queuetest.Embedded`,
`storetest.ForEachDialect` (DB tests run on SQLite and, under make test-pg, Postgres).

## Exported API

```go
package processor

type Config struct {
    Concurrency      int           // WORKER_CONCURRENCY
    BatchSize        int           // WORKER_BATCH_SIZE
    FetchMaxWait     time.Duration // WORKER_FETCH_MAX_WAIT
    MaxDeliver       int           // QUEUE_MAX_DELIVER
    MaxMessageChars  int
    MaxMetadataBytes int64
    RetentionWindow, RetentionCheckInterval time.Duration
    NakDelay         time.Duration // base for NakWithDelay; ConfigFrom sets DefaultNakDelay (1s, §7)
}
const DefaultNakDelay = time.Second
func ConfigFrom(cfg config.Config) Config

type Processor struct{ /* unexported */ }
func New(db *store.DB, js jetstream.JetStream, cons jetstream.Consumer, cfg Config, log *slog.Logger) *Processor
// Run starts Concurrency worker goroutines and the retention loop, and blocks until ctx is done and all
// of them have returned. A batch already fetched is finished (insert + acks/naks/DLQ) on a context
// detached from ctx (context.WithoutCancel + a drainTimeout const), so shutdown never strands messages
// half-handled; nothing new is fetched after ctx is done.
func (p *Processor) Run(ctx context.Context) error
// RunRetention deletes rows with received_at < now-RetentionWindow in batches of retentionBatch (1000),
// one transaction per batch, until a batch deletes fewer than 1000. Returns the total.
func (p *Processor) RunRetention(ctx context.Context, now time.Time) (int64, error)

// Normalise turns one message into a row. Errors mean "unparseable" (goes to the DLQ).
func Normalise(h nats.Header, body []byte, cfg Config) (store.FeedbackRow, error)
```

## Worker loop (one goroutine each)

1. `cons.Fetch(BatchSize, jetstream.FetchMaxWait(FetchMaxWait))`; collect `Messages()`; check
   `batch.Error()` (a plain timeout / no messages is normal; check what nats.go v1.55 returns). Fetch
   errors other than that: log at warn and wait `fetchErrorBackoff` (named const) or until ctx is done.
2. For each message `Normalise`. On error: publish to `queue.SubjectDLQ` with a copy of all original
   headers plus `FB-Error: <reason>` (sync `js.PublishMsg`), then `msg.Term()`. If the DLQ publish
   fails, `Nak` instead (never drop, never Term without a DLQ copy).
3. Good rows: `BeginTx` → `store.InsertFeedbackBatch` → `Commit`. Duplicates count as success.
4. Success → plain `Ack` for every good message. Failure → rollback, then two classes:
   - **Database unreachable** (`transient`: begin/commit failure, `ErrConnDone`, `ErrBadConn`, deadline/cancel,
     any `net.Error`, or any other insert error while an immediate ping also fails): never DLQ. Every
     remaining message gets `NakWithDelay(backoff)`, `backoff = min(NakDelay × NumDelivered, 30s)`, and the
     worker pauses that long before fetching again. The consumer has unlimited MaxDeliver, so outages retry forever.
   - **Row rejected** (batch failed, DB reachable): retry each row in its own transaction, Ack those that insert.
     A row that fails alone: `NumDelivered >= MaxDeliver` → DLQ copy with `FB-Error: store: <err>` then `Ack`;
     else `NakWithDelay(backoff)`. The first transient error in this pass makes the rest transient.
   A failed DLQ publish (either path) → `NakWithDelay(backoff)`, never Term/Ack.
Log batch counts at debug; DLQ events at warn (never the message body).

## Normalise rules

Required headers `FB-Tenant`, `FB-Channel`, `FB-Idem`, `FB-Received` (RFC3339Nano → Unix ms); missing or
bad → error. Body: JSON object; `message` string, `strings.TrimSpace`, empty → error, clamp to
MaxMessageChars runes (never split a rune). `userId` (≤ 256 runes), `appVersion` (≤ 64), `platform`
(≤ 32, lower-cased): optional strings, trimmed, empty → NULL, clamped; wrong type → error. `metadata`:
absent/null → NULL; object → re-encoded compact (`json.Compact`); not an object or compact length >
MaxMetadataBytes → error. Clamp limits are named consts. ID = `store.NewID(store.PrefixFeedback)`.
`IdempotencyKey` = FB-Idem header (the body's idempotencyKey is ignored; ingest already copied it).
Message text is kept as given apart from the trim: line breaks preserved, no HTML/Markdown processing.

## Retention loop

Wait a jittered `RetentionCheckInterval` (uniform in [0.9, 1.1] × interval, math/rand/v2), run
`RunRetention(ctx, time.Now())`, log the count at info when > 0, repeat until ctx is done. Not run at
startup (first run after one interval).

## Tests (no sleeps for synchronisation: poll stream/consumer/DB state with a deadline)

normalise_test.go: `TestNormaliseFull` (every field mapped; platform "iOS" → "ios"; metadata
`{ "a" : 1 }` → `{"a":1}`; received ms exact); `TestNormaliseClamps` (message of MaxMessageChars+5
multi-byte runes → exactly MaxMessageChars runes, valid UTF-8; userId 300 → 256); `TestNormaliseTrim`
(message "  hi\nthere  " → "hi\nthere"); `TestNormaliseErrors` table (missing each header, bad
FB-Received, not JSON, array, message missing / whitespace, metadata array, userId number).
processor_test.go (embedded NATS + ForEachDialect): `TestBatchLandsAndAcks` (publish 50 messages with
proper headers, run Processor → 50 rows, consumer NumAckPending 0 and NumPending 0, stream state
unchanged messages acked); `TestRedeliveredDuplicateHarmless` (same FB-Idem published twice with different
Nats-Msg-Id → 1 row, both acked); `TestUnparseableGoesToDLQ` (bad body → one message on FEEDBACK_DLQ
carrying every original header plus FB-Error, no row, consumer has nothing pending, good messages in the
same batch still land); `TestStoreFailureEndsInDLQ` (MaxDeliver 2, NakDelay 10ms, feedback table renamed
away (`ALTER TABLE feedback RENAME TO feedback_off`) → message ends in DLQ with FB-Error starting "store:",
acked, never inserted); `TestShutdownFinishesBatch` (cancel ctx right after rows start landing: Run returns
within 5s and every message is either acked+inserted or still pending for redelivery — none lost:
rows + NumPending + NumAckPending-after-AckWait accounting, simplest: after Run returns, restart a
Processor and all N rows exist); `TestConcurrencyNoDuplicates` (Concurrency 4 on Postgres/2 on SQLite,
500 messages → 500 rows).
retention_test.go: `TestRunRetention` (rows at now-window-1h ×2500, now-window+1h ×3 → deletes 2500 in 3
batches, 3 remain); `TestRetentionLoopFires` (RetentionCheckInterval 50ms → old row disappears; poll).

## Acceptance

`make vet`, `make test`, `make test-pg` green, processor tests passing on both dialects. No message is
ever Termed or Acked without being inserted or copied to the DLQ. No quota/billing/key code. All
durations from Config or named consts. Run leaves no goroutine behind.
