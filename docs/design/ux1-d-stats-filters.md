# UX phase 1, brief D: filter-aware stats

Author: Opus director · Milestone UX phase 1 · Source (binding): `docs/design/ux-review/VERDICT.md` "Do, Phase 1" item 5 ("Honest tab counts. Extend `/api/stats` to accept the list filters") and "Decisions the reviewers asked for" (counts under filters come from the backend, not hiding). Architecture: `docs/ARCHITECTURE.md` (packages, allowed dependencies), `docs/API.md` (List filters, Stats).

Briefs A to C are done. The UI already sends the active non-status filters to `GET /api/stats` (`web/src/lib/endpoints.ts` `stats(filters)`: `tag`, `channel`, `userId`, `q`); the server ignores them today. This brief makes the server honour them. No UI code change is expected except the one test in §4.

No new Go dependency. Work only in `/Users/njkuhne/Projects/FeedBaxterFree-ux`; scratch under `bin/`. No git commands. Go is not installed on the Mac: use the make targets (they run Go in Docker).

## 1. Behaviour

`GET /api/stats` reads the same filter parameters as the list (`tag`, `channel`, `userId`, `q`, `from`, `to`), with the same meaning and validation (API.md "List filters"): `q` over 200 characters → 400 "Search text is too long."; malformed `from`/`to` → 400 with the existing messages; `from` after `to` → 400. They combine with AND.

`status` is **not** applied to stats: the response's purpose is per-status counts, so a `status` parameter is ignored (documented). `limit` and `cursor` are ignored.

Response shape unchanged: `total`, `byStatus` (all four keys present in the store result; API omits nothing new), `byChannel`, `byTag`, all computed over the filtered set. With `tag=bug`, `byTag` lists every tag that co-occurs on the matching items (bug included); with `channel=X`, `byChannel` has at most that channel. That is the honest meaning; document it in one sentence.

The MCP `stats` tool keeps its current inputs (`from`, `to`); it calls the new service signature with a filter carrying only those. No MCP schema change.

## 2. Code

- `internal/store/feedback_query.go`: `GetFeedbackStats(ctx, q, d, tenantID string, f FeedbackFilter) (FeedbackStats, error)` replaces the `from, to *int64` parameters; it builds its WHERE with `feedbackWhere(tenantID, f)` after clearing `f.Statuses` (stats never filter by status, enforced here as well). The three queries keep their shape; the tag query's `WHERE` already uses the alias `f`, so the tag `EXISTS` subquery and `q` LIKE work unchanged. Both dialects: only `?` placeholders through `d.Rebind`, no dialect-specific SQL.
- `internal/service/feedback.go`: `Stats(ctx, p, f Filter) (store.FeedbackStats, error)`; validate and convert with the existing `f.toStore()` (which validates statuses, `q` length and range), then clear `Statuses`.
- `internal/api/feedback.go` `stats`: use `filterFrom(r)` (the shared parser, already named "shared by list, export and stats"), then pass it on. Drop the `status` values before calling (ignored, never validated, so `status=bogus` on stats is not a 400; say so in API.md).
- `internal/mcp/tools.go`: `h.svc.Stats(ctx, principal(ctx), service.Filter{From: from, To: to})`.
- Update every caller and test helper that used the old signatures (`internal/service/*_test.go`, `internal/store/feedback_query_test.go`, `internal/mcp/mcp_test.go`, roles tests).
- `docs/API.md`: the List filters intro becomes "`GET /api/feedback`, `GET /api/feedback/export` and `GET /api/stats` use these query parameters (stats ignores `status`, `limit` and `cursor`)"; the Stats table row becomes `GET /api/stats[?tag=&channel=&userId=&q=&from=&to=]`; replace "Only `from` and `to` are read…" with the behaviour in §1, including the `byTag`/`byChannel` sentence.

## 3. Tests (Go; names are binding)

`internal/store/feedback_query_test.go` (runs on SQLite and, with `TEST_DATABASE_URL`, Postgres, like the existing store tests):
- `TestFeedbackStatsFilters` with subtests `tag`, `tag_case_insensitive`, `channel`, `userId`, `q`, `q_escapes_like_wildcards` (a `%` in q matches a literal percent only), `range`, `combined` (tag + channel + q + range together), `status_ignored`, `tenant_scoped` (another tenant's matching rows never counted). Each asserts `Total`, all four `ByStatus` values, and for `tag` and `channel` the `ByTag`/`ByChannel` contents.
- Keep `TestFeedbackStats` passing (adapted to the new signature).

`internal/service/feedback_test.go`:
- `TestStatsFilters`: the filter reaches the store (counts change with `tag`, `channel`, `userId`, `q`), `q` over 200 characters is an invalid-request error, `from` after `to` is an invalid-request error, a status in the filter is ignored.

`internal/api/api_test.go`:
- `TestStatsFilterParams`: `GET /api/stats?tag=bug&channel=<id>&userId=u1&q=safari` returns only the matching counts; `?q=<201 chars>` is 400 `invalid_request`; `?status=bogus` is 200 and equals the unfiltered response; the JSON shape is unchanged (keys `total`, `byStatus`, `byChannel`, `byTag`, arrays never null).

`web/src/pages/Inbox.test.tsx` (the one UI test):
- `tab counts update when a tag is chosen`: choosing a tag in the Tag menu triggers `GET /api/stats?tag=bug` and the tabs then show the counts from that response.

## 4. Acceptance

Paste the tails of:
```
make vet
make test
make test-pg        # Postgres from deploy/compose.dev.yml; it tears itself down
cd web && npm ci && npm run typecheck && npm test && npm run build
```
All green. No change outside the files named in §2 and §3. No git commands. Report every deviation.
