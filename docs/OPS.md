# Operating FeedBaxter

## Throughput

Measured with the benchmarks in `internal/ingest/bench_test.go` and `internal/processor/bench_test.go`.

Host: Apple M5 Mac, Docker Desktop 29.9.2 Linux VM (kernel 7.0.14-linuxkit, 10 CPUs, 7.7 GiB), container `golang:1.27-alpine` (go1.27.3, linux/arm64, `nproc` 10). Numbers from a laptop VM are indicative, not a capacity guarantee.

Command (inside the Go container, three runs each):

    go test -run '^$' -bench 'BenchmarkIngest|BenchmarkProcessor' -benchtime 300000x -count 3 ./internal/ingest/ ./internal/processor/
    go test -run '^$' -bench 'BenchmarkProcessor/postgres' -benchtime 100000x -count 3 ./internal/processor/   # with TEST_DATABASE_URL set

| Benchmark | Result (3 runs) | Notes |
| --- | --- | --- |
| Ingest over HTTP (loopback) | 94.3k to 98.0k req/s; p50 374 to 381 µs; p99 1.4 to 1.6 ms | 40 concurrent clients, ~300-byte body, synchronous publish, counting on |
| Ingest handler only (no TCP) | 124.4k to 126.3k req/s; p50 280 to 293 µs; p99 1.0 to 1.1 ms | `ServeHTTP` on a recorder |
| Processor, SQLite, batch 100 | 21.3k to 23.7k msg/s | concurrency 1 |
| Processor, SQLite, batch 500 (default) | 26.4k to 27.4k msg/s | concurrency 1 |
| Processor, Postgres, batch 500 | 99.5k to 100.5k msg/s | concurrency 4 |

Ingest is bounded by HTTP handling plus the synchronous publish to the stream. The processor is bounded by database write speed (SQLite allows one writer), which is why batching helps. The queue between them absorbs bursts: ingest can run faster than the processor until the stream reaches its cap. The benchmark disables the per-key rate limit; with the default (60 per 10 s) one key is held to about 6 requests per second.

## Queue objects

The embedded NATS store lives in `$DATA_DIR/nats`. It must be on the same persistent volume as the SQLite file; losing it drops queued, unprocessed feedback.

| Object | Kind | Configuration | Holds |
| --- | --- | --- | --- |
| `FEEDBACK` | stream, subject `feedback.ingest` | file storage; `QUEUE_MAX_MSGS`, `QUEUE_MAX_BYTES`; discard-new when full; 2 min duplicate window | work queue: accepted submissions until they are inserted and acked (then removed), so the caps bound the backlog, not history |
| `processor` | durable consumer on `FEEDBACK` | explicit ack; unlimited redelivery; ack wait 30 s; max ack pending 2000 | the processor's position |
| `FEEDBACK_DLQ` | stream, subject `feedback.dlq` | file storage; 10 000 messages, discard-old | submissions that failed validation or exhausted redeliveries; `FB-Error` header gives the reason |
| `keys` | KV bucket | file storage; history 1 | ingest key entries by hash (tenant, channel, limits, revoked flag) |
| `counters` | KV bucket | file storage; history 1; per-message TTL (day counters 40 days, month counters 400 days) | per-tenant daily and monthly submission counts |

## Configuration, operationally

Defaults match `.env.example`.

| Variable | Default | Effect of changing it in production |
| --- | --- | --- |
| `MODE` | `all` | `all`, `ingest`, `worker` or `api`. Split modes need `NATS_URL` and a shared database. |
| `HTTP_ADDR` | `:8080` | Listener for UI, API and (in `all`) ingest. |
| `INGEST_ADDR` | `:8081` | Separate ingest listener in `ingest` mode. |
| `DATA_DIR` | `/data` | SQLite file and embedded NATS store. Must be persistent. |
| `DATABASE_URL` | SQLite in `DATA_DIR` | A `postgres://` URL switches to Postgres, which allows several workers. |
| `NATS_URL` | empty | Empty uses the embedded server (`all` only); set it to use an external NATS. |
| `KEY_CACHE_RESYNC_INTERVAL` | `5m` | Safety net that reconciles the `keys` bucket from the database. Revocations reach ingest through the KV watch immediately and do not wait for it. |
| `COUNTER_FLUSH_INTERVAL` | `5s` | Longer means fewer KV writes but more overshoot of daily and monthly limits across replicas. |
| `RETENTION_WINDOW` | `17520h` | Feedback older than this is deleted. |
| `RETENTION_CHECK_INTERVAL` | `1h` | How often retention runs. |
| `DAILY_LIMIT`, `MONTHLY_LIMIT` | `0` (unlimited) | Submission caps; over the cap ingest answers 429. |
| `RATE_LIMIT_PER_KEY`, `RATE_LIMIT_WINDOW` | `60`, `10s` | Per-key burst allowance and refill window; 429 `rate_limited` above it. |
| `MAX_BODY_BYTES` | `16384` | Larger bodies get 413. |
| `MAX_MESSAGE_CHARS` | `5000` | Longer messages are rejected (400). |
| `MAX_METADATA_BYTES` | `4096` | Must be below `MAX_BODY_BYTES`. |
| `QUEUE_MAX_MSGS` | `100000` | Burst headroom. Raising it buys time when processors are behind, at the cost of disk. |
| `QUEUE_MAX_BYTES` | `268435456` | Same, in bytes. Whichever cap is hit first returns 503. |
| `QUEUE_MAX_DELIVER` | `5` | Attempts at inserting a row the database rejects before it goes to the DLQ; database outages retry forever. |
| `WORKER_CONCURRENCY` | 1 (SQLite), 4 (Postgres) | Parallel fetch-and-insert loops. More than 1 on SQLite only adds lock contention. |
| `WORKER_BATCH_SIZE` | `500` | Rows per transaction; larger is faster up to a point (see the table above). |
| `WORKER_FETCH_MAX_WAIT` | `250ms` | How long a worker waits to fill a batch; bounds the added latency when traffic is light. |
| `PUBLISH_TIMEOUT` | `2s` | How long ingest waits for the stream to acknowledge before answering 503. |
| `LOG_LEVEL`, `LOG_FORMAT` | `info`, `text` | `debug|info|warn|error`; `text|json`. |

## Signals

- `503 queue_full` from ingest means the stream is at its cap or unreachable: processors are behind or down. Clients are told to retry.
- Rising oldest-message age means processors cannot keep up: add a worker replica (Postgres only; SQLite has one writer).
- A DLQ count above 0 means inspect `FEEDBACK_DLQ`; the `FB-Error` header on each message says why.
- `/healthz` returns 200 whenever the process serves. `/readyz` returns 503 until startup has finished; for ingest that includes the key-cache replay and loading the counters. Use `/readyz` for load-balancer checks and `/healthz` for liveness.
- Queue depth, oldest-message age and DLQ count will be shown on `/api/ops` (admin only, M2). Until then: depth is the `FEEDBACK` stream message count and the oldest age is the timestamp of its first message (`nats stream info FEEDBACK` against an external NATS).

## First boot

On first start an ingest key is created and logged once, for example `ingest key for channel Default: fbx_…`. Only its hash is stored, so it cannot be retrieved later: copy it from the first-boot log. Key management arrives in M2.

## Shutdown

On SIGTERM the process stops accepting requests, finishes in-flight ones, lets the processor finish its current batch, and flushes counters. Give the container a stop grace period of at least 20 seconds.
