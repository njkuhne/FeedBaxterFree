# Backlog

Deferred items raised during milestone reviews. Not bugs in shipped behaviour; each is a conscious "later".

| Item | Raised in | Notes |
| --- | --- | --- |
| Login rate limiting | M2 | Argon2 concurrency is capped at 2 so a flood cannot exhaust memory, but there is no per-account attempt limit. No client IP is collected (ADR 0005), so a limiter would be per email. Consider before SaaS launch (M6). |
| Non-ASCII case folding on SQLite | M2 | `lower()` in SQLite folds ASCII only, so search and tag matching are case-insensitive for ASCII text only on self-host. Postgres is fine. |
| Channel names unique case-sensitively, tags case-insensitively | M2 | Align in a migration if it ever bites. |
| Postgres JSONB normalises metadata spacing | M2 | API and export output on Postgres re-formats the customer's JSON whitespace. Harmless; document in API.md if asked. |
| `/api/ops` returns 500 when NATS is unreachable | M2 | Should report an "unavailable" queue section instead. Do with the ops page in M3. |
| Concurrent deletion of the last two channels on Postgres | M2 | Both could pass the "keep one channel" check. Row-lock like the last-admin rule. |
| `ingest` mode against an external NATS is untested | M1 | Embedded tests use `DontListen`. Cover in the M6 SaaS deployment work with compose.dev.yml's nats service. |
| Race detector never run | M1 | `CGO_ENABLED=0` in the container rules out `-race`. Add a CI job with a CGO-enabled toolchain. |
| Per-tenant in-flight cap on the queue | M1 | The stream cap is global; the daily cap and rate limit are the fuse. Revisit for SaaS. |
| New tenants' counters start at 0 until the first flush | M1 | Within the documented overshoot bound; `KeyWriter.SyncTenant` scans every KV key, fine for self-host, needs an index at SaaS scale. |
| SaaS Authenticator needs password helpers | M2 | `ext` exposes no password hashing; the private repo must use argon2id with the same PHC format, or `ext` grows a `PasswordHasher` in M6. |
