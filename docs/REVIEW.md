# FeedBaxter spec review and build plan

Reviewed Oct 31, 2026 · Reviewer: Fable (architecture) · Input: `CLAUDE.md`, `docs/SPEC.md`, `assets/brand/*`

## Verdict

The spec is sound where it matters. The pipeline shape (ingest never touches the database, caps enforced at ingest, bounded queue with `DiscardNew`, idempotent batch processors) is the right design for "take in feedback strings, fast, under load", and the open-core boundary, roles and privacy rules are clear enough to build against.

Five decisions need changing before we start, because they would either block the build or send it off course, and one (the footprint target) I measured and settled. Each has a concrete replacement below. Everything else I treat as settled, with a few routine choices recorded in section 3 so Opus and Sonnet do not have to guess.

## 1. Decisions to change (ranked by impact)

### 1.1 Footprint: embedded JetStream fits, so the target only needs a small revision

I measured this rather than guess. Docker, Go 1.27.3, `CGO_ENABLED=0`, `-s -w -trimpath`, arm64:

| What | Measured |
| --- | --- |
| Binary: nats-server 2.15 with JetStream, nats.go, modernc SQLite, all exercised | 19.5 MB |
| Idle RSS: embedded JetStream running, stream and consumer created, SQLite open | 27 MB |
| RSS right after a 50,000-message burst | 88 MB (file-store cache and heap; settles) |
| Publish 50,000 envelopes in-process | 0.12 s, about 430k msg/s |
| Drain 50,000 envelopes into SQLite, batches of 500, one transaction each | 0.23 s, about 214k msg/s |

Adding pgx, the MCP SDK, the React bundle, subset fonts and our own code puts the shipped binary at roughly 28–32 MB and idle RSS at roughly 35–45 MB with the HTTP servers up. That overshoots the spec's 15–25 MB and 20–30 MB by a margin that does not matter on any VM, and in exchange self-host runs the exact ingest, queue and processor code SaaS runs, including queue-full and redelivery behaviour.

**Decision: keep embedded JetStream. Set the targets to binary ≤ 32 MB and idle RSS ≤ 48 MB, and measure both in CI.** The alternative, a SQLite-backed queue for self-host only, would save about 10 MB of binary and 15 MB of RAM in exchange for a second queue implementation to test and a standing source of "works on self-host, breaks on SaaS" bugs. Not worth it.

The throughput numbers also settle where the load goes: the processor drains two hundred thousand messages a second on one core, so it is never the bottleneck. HTTP handling at ingest is, and ingest is stateless and scales by replica.

### 1.2 Open core via a Go build tag will not work as described; invert the dependency

The spec has the private SaaS module "compiled in with a Go build tag". Build tags do not solve the import direction: for the public binary to compile the private code in, the public module must *import* the private module, so the public `go.mod` would reference a private repo that nobody outside can fetch, and `go mod tidy` breaks for every contributor.

**Recommendation: the private repo imports the public one, never the reverse.**

- Public module `feedbaxter` exposes `feedbaxter.Run(ctx, Options)` plus a small exported extension surface: `TenantResolver`, `Authenticator`, `LimitsSource`, extra migrations, extra routes, startup hooks.
- Public `cmd/feedbaxter` fills `Options` with the single-tenant resolver and self-host auth.
- Private repo has `cmd/feedbaxter-saas` which imports the public module and fills `Options` with the multi-tenant resolver, email auth, Stripe, plans and super-admin. No build tags, no private paths in the public tree.

This forces the extension interfaces to be designed in M1–M2 rather than discovered in M6, which is a feature.

### 1.3 "Plans and quotas live in the private layer" contradicts "all caps enforced at ingest"

Ingest is public and must enforce daily and monthly limits. So **enforcement is core; management is private.** The key-cache entry carries `daily_limit` and `monthly_limit` as plain numbers. Self-host sets them from config (default unlimited; an optional `DAILY_LIMIT` is cheap and useful against runaway loops, which the spec itself names as the daily cap's purpose). Plans, Stripe, super-admin and the code that *decides* the numbers stay private. The spec wording should say this.

### 1.4 Key cache: three sync mechanisms where one will do

The spec pushes key events over NATS core pub/sub, resyncs every 5 minutes, and does a synchronous fallback lookup to the BFF on a cache miss. NATS core publish is fire-and-forget: a lost revoke leaves a revoked key live for up to 5 minutes, and the fallback makes ingest depend on the API process being up on every unknown key.

**Recommendation: one NATS KV bucket, `keys`, as the ingest-facing source of truth.**

- The API writes an entry on every key create, revoke, plan change and limit change: `sha256(key) → {tenant, channel, key_id, daily_limit, monthly_limit, revoked}`.
- Ingest *watches* the bucket. Startup replay fills the cache (and gates readiness); live updates arrive as they happen. KV is JetStream-backed, so nothing is lost.
- The 5-minute job becomes a DB → KV reconcile in the API, the safety net the spec wanted. `KEY_CACHE_RESYNC_INTERVAL` keeps its name and default.
- Unknown key means reject with `invalid_key`. No fallback call, no negative cache, no runtime dependency on the API.

### 1.5 Drop "separate schemas for feedback and account tables"

SQLite has no schemas. Honouring this creates a dialect split for a migration that may never happen. Plain table names in one schema in both databases. If the feedback tables ever need their own database, that is a one-time migration then, not a tax now.

### 1.6 Drop the client IP from the queue envelope

The spec stores client IP in the envelope but no table persists it, and the rate limit is per key, not per IP. Collecting personal data that nothing uses contradicts "avoids holding personal data by design" and makes the GDPR story harder. Remove it. If abuse control ever needs per-IP limiting it happens in memory at ingest and is never written anywhere.

## 2. Things to fix in the spec text (not blockers)

- **Idempotency key must be optional.** The onboarding flow is "copy the key, make one curl call". Nobody types a UUID into curl. Absent key: ingest generates one, and the dedupe guarantee then only covers SDK retries. Say so.
- **CORS is missing.** Publishable keys from a script tag mean `POST /v1/feedback` is cross-origin from any site, with an `Authorization` header, so every send is preceded by a preflight. Ingest answers `OPTIONS` from memory with a long `Access-Control-Max-Age`. Add to the ingest section.
- **Dead-letter is not automatic.** JetStream does not move messages to a DLQ. The processor does it: on the final allowed delivery, publish to a small `feedback.dlq` stream and ack. Unparseable payloads go there on first sight.
- **Multi-replica counters need CAS.** "Overshoot of one flush interval per replica" holds only if each flush does read, add local delta, compare-and-swap write on a shared KV key. Per-replica keys summed would not give that bound. Also the per-key rate limit is per replica, so three ingest replicas allow three times the stated rate; document it.
- **MCP transport is unspecified.** The server runs inside the API binary, so it is remote: Streamable HTTP at `/mcp`, `Authorization: Bearer <personal access token>`. Stdio is not an option for a hosted server.
- **The queue cap is global.** One tenant can fill the stream and cause `queue_full` for everyone. The daily cap and rate limit are the fuse; a per-tenant in-flight cap is a v2 item. Note the risk.
- **Rate limiter and counters are in memory per replica**, so the "very next request" guarantee is per replica. The spec already says this for counters; say it once, clearly, for both.
- **Fraunces is a large variable font.** Subset to Latin, weight 400, the `opsz` axis only. Inter: Latin, 400 and 500. Otherwise fonts alone add ~1.5 MB to the binary.

## 3. Choices I am making as architect

Recorded so the design briefs are consistent. Push back on any of them.

| Area | Choice | Why |
| --- | --- | --- |
| Go toolchain | Go via the official `golang` Docker image, `CGO_ENABLED=0`; a `Makefile` wraps it. Postgres via compose. | Go is not installed on this Mac; Docker is. `brew install go` would make iteration faster and is your call. |
| SQLite driver | `modernc.org/sqlite` 1.61 (pure Go) | CGO SQLite breaks the static scratch build and cross-compiling. Pure Go is ~1.5–2× slower per statement, irrelevant at 214k inserts/s measured. |
| Postgres driver | `pgx/v5` (5.12) in `database/sql` mode | Maintained; `lib/pq` is in maintenance mode. |
| SQL | Hand-written queries, a 5-line dialect shim (placeholders, JSON column type, timestamps). No ORM, no sqlc. | Stdlib preference; the shim is the whole dual-database tax, kept visible. |
| Envelope format | NATS headers for `tenant`, `channel`, `key_id`, `received_at`, `idempotency_key`; raw request body as message data; `Nats-Msg-Id` set to `tenant:idempotency_key`. | Zero re-serialisation on the hot path, and JetStream's dedupe window catches SDK retries before they reach the processor. |
| Publish mode | Synchronous publish with ack and a short timeout. | The ack is how ingest learns the queue is full (503 `queue_full`). Async would lie with a 202. |
| HTTP | `net/http` with Go 1.22+ pattern routing, `http.MaxBytesReader`. No framework. | Stdlib. |
| Auth sessions | Server-side session table, `HttpOnly` `SameSite=Lax` cookie, `Sec-Fetch-Site` check on mutations. Passwords: argon2id. | Simple, no JWT key management. |
| MCP | Official `modelcontextprotocol/go-sdk` 1.8, Streamable HTTP, PAT bearer. | One dependency; hand-rolling the protocol is not worth it. |
| Retention | Runs on a jittered ticker in every `worker` replica; batched deletes are idempotent so overlap is harmless. | No leader election needed. |
| Metrics | A JSON ops endpoint and the internal ops page; no Prometheus client in v1. | Keeps dependencies down; add an exporter later if wanted. |
| Repo | `/Users/njkuhne/Projects/FeedBaxter` is the public repo root; `CLAUDE.md`, `docs/`, `assets/` move there from the "feedbaxter Instructions" folder; `git init`. | The folder name has a space and the spec's relative paths assume it is the root. |
| Dependencies (public) | `nats-server/v2` 2.15, `nats.go` 1.55, `modernc.org/sqlite`, `pgx/v5`, `x/crypto` (argon2), `modelcontextprotocol/go-sdk`. Go 1.27. That is the list. | Anything else needs a reason in an ADR. |

## 4. Target architecture

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
`all` mode = the three above in one process + embedded nats-server (in-process connection, no TCP).
```

**Package layout (public repo)**

```
cmd/feedbaxter/        serve --mode, migrate, reset-admin
feedbaxter.go          Run(ctx, Options); the extension surface the private repo uses
ext/                   TenantResolver, Authenticator, LimitsSource, Deps (exported)
internal/config        env → typed config, Go durations
internal/queue         embedded server, streams, consumers, KV buckets
internal/ingest        handler, keycache, limiter, counters, publisher
internal/processor     consumer loop, batch insert, DLQ, retention
internal/store         open, dialect, migrations, queries
internal/api           router, sessions, handlers, export
internal/mcp           tools
internal/ops           health, readiness, metrics
web/                   React + Vite admin UI, built into web/dist, embedded
sdk/web, sdk/swift
deploy/                Dockerfile (scratch), compose (self-host; Postgres+NATS for SaaS dev)
docs/                  SPEC.md, REVIEW.md, ARCHITECTURE.md, design/<component>.md, adr/
```

## 5. Build plan

Milestones are the unit of acceptance. Each one ends with a working, tested slice, not a layer.

| # | Milestone | Done when |
| --- | --- | --- |
| M0 | Repo and toolchain | `git init` at the project root, instructions moved, `make test` runs Go tests in Docker against SQLite and Postgres, Dockerfile builds a scratch image, `ARCHITECTURE.md` and ADRs for section 1 and 3 committed. |
| M1 | Pipeline core | `curl` → 202 → row in SQLite in `all` mode. Tests: invalid key, payload too large, rate limit, daily and monthly caps with CAS flush, `queue_full` under `DiscardNew`, idempotent redelivery, DLQ after max deliveries, KV key watch (create, revoke, limit change). A load number recorded in the ops doc. |
| M2 | Accounts and API | First-boot tenant and one-time admin password, forced change, users and roles, sessions, channels and keys (written to KV), PATs, feedback list/filter/search, status with history, tags, bulk, export, per-user delete, retention, ops endpoint. API tests for every role rule in the spec table. |
| M3 | Admin UI and self-host image | All v1 screens to the Styleguide, fonts subset and embedded, Baxter where the spec allows, keyboard and WCAG AA checks, `reset-admin`, compose file. **Self-host v1 ships here.** |
| M4 | MCP server | All seven tools, role-limited, `via: mcp` in history, tested with a real MCP client. |
| M5 | SDKs | Web (npm and script tag) with retry, backlog and `plan_limit` stop; then Swift package with disk-backed retry. |
| M6 | SaaS layer (private repo) | Multi-tenant resolver, sign-up and email, Stripe, plans, super-admin, deployment manifests for standalone NATS and Postgres. ToS and privacy policy are yours and a lawyer's. |

## 6. How Fable, Opus and Sonnet work on this

- **Fable (this session):** owns the architecture, ADRs and milestone acceptance. Writes the `ARCHITECTURE.md` the briefs derive from, resolves cross-component conflicts, runs the acceptance checks at the end of each milestone, and reports to you.
- **Opus (director):** per milestone, writes one design brief per component into `docs/design/` (exported API, types, file list, tests to write, acceptance criteria, and the spec non-negotiables that apply). Spawns a Sonnet coder per brief, reviews each diff against the brief and the spec, sends it back until it passes, then hands the milestone to Fable.
- **Sonnet (coder):** implements exactly one brief at a time on its own branch, runs the tests in Docker, and reports back with the test output. Does not change interfaces without going back to Opus.

Two coders never touch the same package at once. Nothing merges without an Opus review and green tests on both databases.

## 7. Decisions I need from you

1. Section 1.2: agree that the private repo imports the public module (no build tags).
2. Sections 1.3–1.6: OK to amend the spec as described, and to fold section 2 into it.
3. Repo: OK to `git init` at `/Users/njkuhne/Projects/FeedBaxter` and move `CLAUDE.md`, `docs/` and `assets/` to the root.
4. Toolchain: Docker-only Go is fine and already proven by the probe. If you run `brew install go` the loop gets faster; your call, not a blocker.

Say "proceed" and I start M0 with the recommendations as written.
