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
| "Monthly limit reached" state in the UI | M3 | The empty state exists in the design but no API endpoint exposes the tenant's limits or whether it is capped. Add a limits field to `/api/me` or `/api/ops` with the SaaS work in M6 and wire the state then. |
| Compose build reports `version=dev` | M3 | `deploy/docker-compose.yml` passes no `VERSION` build argument. Pass it from the release workflow once releases exist. |
| Metadata separators read aloud | M3 | The inbox row's "·" separators are plain text; a screen reader reads them. Mark them `aria-hidden` in a polish pass. |
| No per-token rate limit on `/mcp` or `/api` | M4 | Neither endpoint is limited per token or per user. Ingest is limited per key; add the same for authenticated callers before SaaS launch (M6). |
| No CORS on `/mcp` | M4 | Browser-based MCP clients cannot connect. Desktop and CLI clients are unaffected. |
| PAT-only auth on `/mcp`, no OAuth | M4 | Clients that require OAuth discovery cannot connect. Revisit if customers ask. |
| No export tool over MCP | M4 | `list_feedback` pages instead (up to 200 per call). The UI and `GET /api/feedback/export` cover full exports. |
| No request logging on `/mcp` | M4 | Only errors are logged, with no tokens or inputs. Add a quiet access line (tool name, status, duration) if operators need it. |
| Serve `feedbaxter.js` from the binary | M5 | Serve it (for example at `/sdk/feedbaxter.js`) so self-hosters need no CDN. |
| No send on page unload | M5 | No `keepalive` fetch or `sendBeacon`; a submission made just before the tab closes can be lost if its first attempt has not finished. |
| No periodic backlog timer | M5 | Queued items retry on the next `send`, `online` event or page load only. |
| Web SDK tabs share one backlog without locking | M5 | Concurrent flushes can send an item twice (deduped by the server) and in a rare write race one tab's addition can be lost. |
| npm publishing not set up | M5 | Scope `@feedbaxter`, provenance and a release workflow are still to do. |
| Swift SDK | M5 | M5 part 2. |
| Web SDK backlog is not keyed by server `url` | M5 | Items store the ingest key but not the origin, so two clients on one page pointed at different FeedBaxter servers share a backlog and a queued item can be flushed to the other server. One app talks to one server, so this is left as is. |

## UX phase 3

- Auto-advance toggle on the detail page. Phase 1 advances by default after a decision that takes the item out of the list it was opened from, with Undo as the safety net. A switch belongs in the phase 3 shortcuts dialog.
- Single-key shortcuts have no off switch (WCAG 2.1.4); phase 1 added `z`, `u`, `t` and Shift+`x`. Put the switch in the same shortcuts dialog.
- The Undo toast lasts 6 s and `z` works only while it is visible. Revisit the duration after real use.
- Tag and Channel menu counts are totals (from `/api/tags` and the first unfiltered stats load), not counts under the other active filters.
- Export still has no error path in the UI (phase 3 item 12).
