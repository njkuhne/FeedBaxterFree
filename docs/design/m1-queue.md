# M1 brief: `internal/queue` and `internal/queue/queuetest`

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §1, §6, §11, §12; ADR 0001, 0003

Start the embedded nats-server (mode `all`) or connect to `NATS_URL` (other modes), and declare every
JetStream object idempotently. Everything else in M1 (ingest, processor, keysync, wiring) builds on this.
`docs/probe/main.go` is a working example of the exact API calls at the pinned versions; reuse it.

**go.mod.** You are the only M1 coder allowed to touch go.mod. Inside the container:
`go get github.com/nats-io/nats-server/v2@v2.15.2 github.com/nats-io/nats.go@v1.55.0`, write the code,
then `go mod tidy`. Afterwards go.mod must list exactly four direct requirements: pgx, modernc sqlite,
nats-server/v2, nats.go. No other new module.

**Files.** `internal/queue/queue.go` (names, Limits, Conn, Start, Connect, Close), `ensure.go` (Ensure,
Objects), `errors.go` (IsQueueFull, IsWrongLastSequence), `kv.go` (PutTTL), `queuetest/queuetest.go`;
tests `queue_test.go`, `ensure_test.go`, `kv_test.go`.

## Exported API

```go
package queue

const (
    StreamFeedback    = "FEEDBACK";     SubjectIngest = "feedback.ingest"
    ConsumerProcessor = "processor"
    StreamDLQ         = "FEEDBACK_DLQ"; SubjectDLQ    = "feedback.dlq"
    BucketKeys        = "keys";         BucketCounters = "counters"

    HeaderTenant = "FB-Tenant"; HeaderChannel = "FB-Channel"; HeaderKey = "FB-Key"
    HeaderReceived = "FB-Received"; HeaderIdem = "FB-Idem"; HeaderError = "FB-Error"
)

// Fixed by ARCHITECTURE §5/§6 (not env vars). Named here so nothing else repeats a literal.
const (
    DuplicateWindow  = 2 * time.Minute
    AckWait          = 30 * time.Second
    MaxAckPending    = 2000
    DLQMaxMsgs       = 10000
    CounterDayTTL    = 40 * 24 * time.Hour
    CounterMonthTTL  = 400 * 24 * time.Hour
    CounterMarkerTTL = time.Hour // LimitMarkerTTL on `counters`; enables per-message TTL
)

// Limits are the configurable stream/consumer caps (QUEUE_MAX_MSGS, QUEUE_MAX_BYTES).
type Limits struct{ MaxMsgs, MaxBytes int64 }
func LimitsFromConfig(cfg config.Config) Limits

// Conn is a NATS connection plus JetStream context; it owns the embedded server when there is one.
type Conn struct {
    NC *nats.Conn
    JS jetstream.JetStream
    // unexported: *server.Server (nil when connected to NATS_URL)
}
// Start runs an embedded nats-server: server.Options{JetStream: true, StoreDir: <cfg.DataDir>/nats,
// DontListen: true, NoSigs: true, NoLog: true}, waits ReadyForConnections, connects with
// nats.InProcessServer. Errors if it is not ready in readyTimeout (unexported const, 10s).
func Start(ctx context.Context, cfg config.Config) (*Conn, error)
// Connect dials cfg.NATSURL (nats.Name("feedbaxter-"+cfg.Mode), nats.MaxReconnects(-1)).
func Connect(ctx context.Context, cfg config.Config) (*Conn, error)
// Close drains the connection (falls back to Close if Drain errors), then shuts the embedded server
// down and waits for it (WaitForShutdown). Safe to call twice.
func (c *Conn) Close()

// Objects are the handles Ensure declared.
type Objects struct {
    Stream, DLQ jetstream.Stream
    Consumer    jetstream.Consumer
    Keys, Counters jetstream.KeyValue
}
// Ensure declares exactly the objects of ARCHITECTURE §6, idempotently (CreateOrUpdateStream,
// CreateOrUpdateConsumer, CreateOrUpdateKeyValue), and returns their handles.
func Ensure(ctx context.Context, js jetstream.JetStream, lim Limits) (*Objects, error)

// IsQueueFull reports whether err is the FEEDBACK stream refusing a message under DiscardNew because
// MaxMsgs or MaxBytes is reached. Verify the server's error code/description against the nats-server
// v2.15.2 source in /go/pkg/mod (ErrMaxMsgs / ErrMaxBytes wrapped in a JetStream API error) and
// match on *jetstream.APIError ErrorCode + those cases, not on a bare string compare of err.Error().
func IsQueueFull(err error) bool
// IsWrongLastSequence reports a CAS conflict (API error codes JSErrCodeStreamWrongLastSequence and
// JSErrCodeStreamWrongLastSequenceConstant; also true for jetstream.ErrKeyExists and ErrKeyRevisionMismatch).
func IsWrongLastSequence(err error) bool

// PutTTL is a compare-and-swap write that carries a per-message TTL. nats.go v1.55's KeyValue.Update
// cannot set a TTL (only Create with jetstream.KeyTTL can), so counters use this for every write.
// lastRev == 0: kv.Create(ctx, key, value, jetstream.KeyTTL(ttl)) (handles a prior delete/expiry marker).
// lastRev > 0: js.PublishMsg on "$KV.<bucket>.<key>" with WithExpectLastSequencePerSubject(lastRev)
// and WithMsgTTL(ttl). Returns the new revision. Conflicts satisfy IsWrongLastSequence.
func PutTTL(ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue, key string,
    value []byte, lastRev uint64, ttl time.Duration) (uint64, error)
```

**Object configs (exactly).**
- `FEEDBACK`: Subjects `[feedback.ingest]`, MaxMsgs/MaxBytes from Limits, `Discard: DiscardNew`,
  `Storage: FileStorage`, `Duplicates: DuplicateWindow`, `Retention: WorkQueuePolicy` (acked messages leave the stream; with
  LimitsPolicy + DiscardNew processed messages would fill it and ingest would 503 forever — found in review).
- `processor`: Durable `processor`, `AckExplicitPolicy`, `MaxDeliver: -1` (unlimited, so a database outage never strands a message; the processor dead-letters rows the database rejects after `QUEUE_MAX_DELIVER` attempts), AckWait, MaxAckPending,
  `DeliverAllPolicy`, FilterSubject `feedback.ingest`.
- `FEEDBACK_DLQ`: Subjects `[feedback.dlq]`, MaxMsgs DLQMaxMsgs, `DiscardOld`, FileStorage.
- KV `keys`: History 1, FileStorage. KV `counters`: History 1, FileStorage, `LimitMarkerTTL: CounterMarkerTTL`
  (this is what sets AllowMsgTTL on the backing stream; confirm in nats.go kv.go).

## queuetest

```go
// Embedded starts an embedded server with StoreDir in t.TempDir(), runs Ensure with lim (zero fields
// default to MaxMsgs 100000, MaxBytes 256 MiB) and registers Close in t.Cleanup.
func Embedded(t testing.TB, lim queue.Limits) (*queue.Conn, *queue.Objects)
```
It builds a `config.Config` with only the fields Start needs (DataDir, Mode "all"); no env reads.
`testing.TB` so benchmarks can use it.

## Tests (no sleeps; poll with a deadline only where the server's own timer is the thing under test)

- `TestEnsureIdempotent` – Ensure twice on one server: no error; stream, consumer and both buckets
  report the configs above (MaxMsgs, Discard New, Duplicates 2m, MaxDeliver -1, AckWait, MaxAckPending,
  DLQ DiscardOld + 10000, KV history 1, counters LimitMarkerTTL > 0). Ensure again with a larger MaxMsgs
  updates the stream in place.
- `TestStreamRejectsWhenFullMsgs` – MaxMsgs 3: three publishes succeed, the fourth returns an error
  with `IsQueueFull(err)==true`; the stream still holds messages 1..3 (DiscardNew kept the old ones).
- `TestStreamRejectsWhenFullBytes` – small MaxBytes: publishing past it gives IsQueueFull true.
- `TestIsQueueFullFalse` – nil, context.DeadlineExceeded, a random error → false.
- `TestDuplicateWindow` – two publishes with the same `Nats-Msg-Id`: second PubAck has Duplicate=true,
  stream has one message.
- `TestPutTTL` – rev 0 creates; stale rev fails with IsWrongLastSequence; correct rev succeeds; the raw
  last message for the subject on stream `KV_counters` carries a `Nats-TTL` header (read it with
  `Stream.GetLastMsgForSubject`). Second rev-0 create on an existing key → IsWrongLastSequence.
- `TestPutTTLExpires` – ttl 1s (server minimum): Get returns `jetstream.ErrKeyNotFound` within 10s
  (poll with deadline); then PutTTL with rev 0 succeeds again (create after expiry marker).
- `TestStartPersistsAndCloses` – Start with DataDir=t.TempDir(): `<dir>/nats` exists; publish one
  message; Close; Start again on the same dir; Ensure; stream has 1 message. Close twice does not panic.
- `TestConnectBadURL` – Connect to `nats://127.0.0.1:1` fails with an error (use a short-timeout ctx or
  nats.Timeout via an unexported option; must return in < 5s).

## Acceptance

1. `make vet` and `make test` green; paste output. `go list -m all | grep nats` shows v2.15.2 and v1.55.0.
2. No literal duration outside the const block; Limits come from config only.
3. Touches only go.mod, go.sum and `internal/queue/...`.
4. No goroutine is left running after Close (the server's WaitForShutdown returns).

Non-negotiables that apply: the main queue is `DiscardNew` and never rolls (only the DLQ is DiscardOld);
no test needs a running NATS (embedded, `DontListen: true`, temp store dir).
