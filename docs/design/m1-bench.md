# M1 brief: benchmarks and `docs/OPS.md`

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §1, §6, §7, §11; SPEC "Operational metrics"; ADR 0001

Measure the two halves of the pipeline and write the operator note. Code changes are test files only.

**Files.** `internal/ingest/bench_test.go`, `internal/processor/bench_test.go`, `docs/OPS.md` (new).
go.mod frozen; no production code changes (if a benchmark exposes a real bug, stop and report it).

## BenchmarkIngest (`internal/ingest`)

- `queuetest.Embedded(b, queue.Limits{MaxMsgs: 50_000_000, MaxBytes: 8 << 30})` so the stream never
  fills during the run (a 503 must fail the benchmark, not be counted).
- One key entry Put into the real `keys` KV; `KeyCache.Run` in a goroutine; wait on `Ready()`.
  Limiter with `perKey` larger than any b.N (e.g. 1<<30) and a 10s window; Counters with no limits and
  `Counters.Run` on a 5s flush (counting stays on, as in production).
- `httptest.NewServer(NewHandler(...))`; one `http.Client` whose Transport has `MaxIdleConnsPerHost`
  ≥ 2×GOMAXPROCS. `b.SetParallelism(4)`; `b.RunParallel` POSTs a realistic body (~300 bytes: message
  with a line break, userId, appVersion, platform, small metadata) with a unique `idempotencyKey` per
  request (atomic counter); every response must be 202 or `b.Error`. Drain and close bodies.
- `b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "req/s")`; also report p50 and p99 latency in µs
  (collect durations per goroutine into slices, merge after; `"p50-µs"`, `"p99-µs"`).
- Sub-benchmark `BenchmarkIngest/handler-only` that calls `ServeHTTP` on an `httptest.NewRecorder`
  in RunParallel (no TCP), to separate handler+publish cost from HTTP overhead.

## BenchmarkProcessor (`internal/processor`)

- SQLite via a temp-dir `store.Open` + `store.Migrate` (not storetest: benchmarks take `*testing.B`;
  if storetest only accepts `*testing.T`, open directly) with one tenant row created via store.
- Setup outside the timer: publish b.N messages with correct headers (sync `PublishMsg` in a loop, or
  PublishAsync + `PublishAsyncComplete` — setup is not the ingest path) using the same ~300-byte body.
- `b.ResetTimer()`; run `processor.New(...).Run` with Concurrency 1, BatchSize 500, FetchMaxWait 250ms;
  poll `SELECT COUNT(*)` every 10ms until it equals b.N; `b.StopTimer()`; cancel and wait for Run.
- Report `msg/s`. Sub-benchmarks `batch=100`, `batch=500` (default).
- If `TEST_DATABASE_URL` is set, also a `postgres` variant with Concurrency 4 (skip otherwise).

## Running and recording

Run inside the Go container, e.g.
`docker run … golang:1.27-alpine go test -run '^$' -bench 'BenchmarkIngest|BenchmarkProcessor' -benchtime 20000x -count 3 ./internal/ingest/ ./internal/processor/`
(pick `-benchtime` so each run takes ≥ 2s). Also run the Postgres variant under the compose network.
Record: host (from `uname -m`, `nproc` inside the container, and Docker's CPU/memory limit from `docker info`),
Go version, exact command, and the raw `go test` output lines.

## docs/OPS.md contents (concise, operator-facing, plain prose and tables)

1. **Throughput** – the numbers above with command and host; one sentence on what they mean (ingest is
   bounded by HTTP + synchronous publish; the processor by SQLite write speed; the queue absorbs bursts).
2. **Queue objects** – table of FEEDBACK, `processor`, FEEDBACK_DLQ, KV `keys`, KV `counters` with their
   config (from ARCHITECTURE §6 and `internal/queue` constants) and what each holds. Where the embedded
   store lives (`$DATA_DIR/nats`) and that it needs the same persistent volume as the SQLite file.
3. **Configuration, operationally** – every env var from `internal/config` (match `.env.example`), one
   line each on what changing it does in production (e.g. raising `QUEUE_MAX_MSGS` buys burst headroom at
   the cost of disk; `COUNTER_FLUSH_INTERVAL` trades KV writes for cross-replica overshoot;
   `KEY_CACHE_RESYNC_INTERVAL` is a safety net, revocations propagate through the KV watch immediately).
4. **Signals** – `503 queue_full` means the stream is at its cap (processors behind or down);
   **rising oldest-message age means add a worker replica**; DLQ count > 0 means inspect FEEDBACK_DLQ
   (FB-Error header says why); `/healthz` vs `/readyz` semantics (readyz waits for the key-cache replay).
5. **First boot** – the ingest key is logged once (`ingest key for channel Default: fbx_…`); it is not
   retrievable later (only its hash is stored), so copy it from the first-boot log. M2 adds key management.
6. **Shutdown** – SIGTERM stops accepting, finishes in-flight requests, drains the processor's batch,
   flushes counters; give the container ≥ 20 s stop grace.

Under 150 lines. No marketing tone.

## Acceptance

Both benchmarks run green in the container with no 503s and no errors; the numbers in OPS.md are the
ones pasted in your report; `make vet` and `make test` still green (benchmarks do not run in `make test`).
