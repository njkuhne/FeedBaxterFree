# FeedBaxter

One-way app feedback capture: an ingest API, SDKs for web and iOS/macOS, an admin inbox for triage, and an MCP server so a customer's own AI can triage. Free, open-source, single-tenant self-host, or paid multi-tenant SaaS.

**The product spec is `docs/SPEC.md` and the engineering design is `docs/ARCHITECTURE.md`. Read both before starting any feature.** The spec is the product source of truth; the architecture doc is the engineering source of truth (packages, interfaces, config, subjects, buckets). Decisions and their reasons are in `docs/adr/`. If something in the code disagrees with either, ask before changing any of them.

## Stack

- Go backend: one static binary with modes `ingest`, `worker`, `api`, and `all` (default for self-host).
- NATS JetStream queue (embedded in-process for self-host, standalone for SaaS); NATS KV for the ingest key cache (`keys` bucket, watched by ingest) and quota counters.
- SQLite for self-host, Postgres for SaaS. Same schema and migrations; every table has `tenant_id`.
- React admin UI, compiled into the binary with `go:embed`.
- Self-host ships as a single Docker service (scratch image, binary ≤ 32 MB, ≤ 48 MB RAM idle, both measured by `make check`).
- Toolchain: Go is not installed on the dev Mac. Everything runs through `make` targets that use the official `golang` Docker image with `CGO_ENABLED=0`; Postgres for tests comes from `docker compose`.

## Non-negotiables

- **Never write feedback to the database from the ingest path.** Ingest validates the key from its in-memory cache, checks quota counters, enqueues, and returns `202`. Only processors write feedback. Ingest never calls the API process either: an unknown key is simply invalid.
- **All caps are enforced at ingest** (rate limit, daily, monthly plan limit) so rejection is immediate. Processors know nothing about quotas or billing.
- **The queue rejects new messages when full** (`DiscardNew`). Never drop queued feedback.
- **Processors are idempotent**: unique `(tenant_id, idempotency_key)`; at-least-once delivery.
- **Config intervals are Go durations from env or config**, never hard-coded: `KEY_CACHE_RESYNC_INTERVAL` (default `5m`), `COUNTER_FLUSH_INTERVAL` (default `5s`), retention window (default 2 years).
- **Open core boundary:** the public repo contains the single-tenant `TenantResolver`, simple self-host auth and quota *enforcement*. Multi-tenancy, sign-up and email flows, Stripe billing, plan management and super-admin live in a private repo that *imports* this module and fills `feedbaxter.Options`. No build tags, and never a reference to the private repo in this tree.
- **No AI runs inside FeedBaxter.** AI triage happens only through the MCP server.
- **Roles:** Admin and Reviewer. Reviewers can view, set any status (reviewed, actioned, ignored), tag, export, and use MCP. Only Admins delete feedback, manage channels, keys, people and billing, or delete the org.
- **Privacy:** `userId` is an opaque customer ID, never an email. No client IP is collected or stored anywhere. Feedback text is plain text: preserve line breaks, never render it as Markdown or HTML.

## Design

Follow the Styleguide section of `docs/SPEC.md` exactly: the oat and dusk palette, Fraunces for wordmark and page titles only, Inter for everything else, statuses with icon plus label, and WCAG 2.1 AA throughout. Light mode only. Brand assets are in `assets/brand/` (full mark at 64 px and up, small mark below 64 px). Baxter never speaks; UI copy stays plain.

## Working style

- Build in the order of the v1 checklist in `docs/SPEC.md`, starting with the ingest → queue → processor pipeline.
- Keep dependencies minimal; prefer the Go standard library. The allowed Go dependencies are listed in `docs/ARCHITECTURE.md`; adding one needs an ADR.
- Roles on this build: Fable owns architecture and milestone acceptance, Opus writes component design briefs (`docs/design/`) and reviews every diff, Sonnet implements one brief at a time. Nothing merges without an Opus review and a green `make check` (vet, tests on SQLite and Postgres, UI, SDK, size and memory) run locally. There is no hosted CI, by decision.
- Write tests for the ingest pipeline: cap enforcement, queue-full behaviour, idempotent processing.
