# UX phase 3, brief C: export filenames, key last used, channel last received

Author: Opus director · Milestone UX phase 3 · Sources (binding): `docs/design/ux-review/VERDICT.md` (Phase 3 items 12 and 14: "descriptive export filenames (backend, small)", "per-channel count and last received and key last-used (backend, small)"), `CLAUDE.md` non-negotiables, `docs/ARCHITECTURE.md` (packages, processor §7, allowed dependencies), `docs/API.md`. Briefs A and B (front end) are built; B already reads `lastUsedAt` on keys, `lastReceivedAt` on channels and the export `Content-Disposition` filename.

Go only, both dialects, plus `docs/API.md` and one line in `docs/ARCHITECTURE.md`. No new Go dependency. MCP tools unchanged. Work only in `/Users/njkuhne/Projects/FeedBaxter/free-ux3`; scratch files under its `bin/`. No git commands. Go is not installed on the host: use `make vet`, `make test`, `make test-pg` (Docker).

## 0. Ground rules (review fails on any of these)

- **The ingest handler never writes the database.** It does not change at all in this brief. `last_used_at` is written by the processor, best effort, after its batch is committed.
- A failure to update `last_used_at` never affects feedback: no rollback, no nak, no dead letter, no extra pause. Log it at Debug (Warn at most once per minute is also fine) and move on.
- Processors stay idempotent and know nothing about quotas.
- Same schema on SQLite and Postgres; every query carries `tenant_id`.
- Existing tests keep passing; existing JSON fields keep their names and meaning.

## 1. Export filename (`internal/api/export.go`)

`Content-Disposition: attachment; filename="feedbaxter-<status>[-<tag>]-<YYYY-MM-DD>.<csv|json>"`
- `<status>` is the status filter, or `all` when there is none (the API's `status` parameter may name more than one status if it already accepts that; then join them with `-` in the order given).
- `-<tag>` only when a tag filter is set: the tag lowercased, every run of characters outside `[a-z0-9]` replaced by one `-`, leading and trailing `-` trimmed, cut to 40 characters (trim a trailing `-` again after cutting). If nothing is left, leave the tag part out.
- Date is today in UTC, `2006-01-02`.
- Channel, search and userId filters are not in the name (they can hold personal or long text).
- Write it as a small pure function `exportFilename(f <the filter type filterFrom returns>, format string, now time.Time) string` so it is unit-tested without HTTP.
- Examples: `feedbaxter-all-2026-11-01.csv`, `feedbaxter-new-bug-2026-11-01.json`, `feedbaxter-reviewed-safari-export-2026-11-01.csv` for tag "Safari Export!".

## 2. Key last used

**Migration** `internal/store/migrations/0003_key_last_used.sql` (one file for both dialects, the existing naming scheme):
```sql
ALTER TABLE ingest_keys ADD COLUMN last_used_at BIGINT;
```
(Unix ms like every other time column; nullable.) Check `schema_test.go` / `migrate_test.go` for anything that pins the migration list or the columns, and update it.

**Store** (`internal/store/ingestkey.go`):
- `IngestKey` gains `LastUsedAt *int64`; `ingestKeyCols` and every scan of it include `last_used_at`.
- New:
```go
// TouchIngestKeys sets last_used_at = at for each key in the tenant whose
// last_used_at is NULL or older than at. Unknown or deleted keys are ignored.
func TouchIngestKeys(ctx context.Context, q Querier, d Dialect, touches []KeyTouch) error
type KeyTouch struct{ TenantID, KeyID string; At int64 }
```
  One `UPDATE ingest_keys SET last_used_at = ? WHERE tenant_id = ? AND id = ? AND (last_used_at IS NULL OR last_used_at < ?)` per touch (there are few distinct keys per batch). Never moves the value backwards.

**Processor** (`internal/processor/processor.go`):
- `Config` gains `KeyTouchInterval time.Duration`; `ConfigFrom` sets it to `DefaultKeyTouchInterval = time.Minute` (the same pattern as `NakDelay`). `<= 0` means touch on every batch (tests use that).
- The processor keeps `lastTouch map[string]time.Time` (key id → wall time of its last touch) behind a mutex (workers run concurrently).
- After a batch's successful insert and acks (and in the per-row path, for the rows that were stored), collect for each stored message the key id from the `FB-Key` header (`queue.HeaderKey`; skip when empty) with the tenant from `FB-Tenant` and the message's received time (`row.ReceivedAt`). Per key keep the newest received time. Drop keys touched less than `KeyTouchInterval` ago. Call `store.TouchIngestKeys` once with the rest on the batch's context; on success record the touch time; on error log at Debug and do not record (the next batch tries again).
- Use `row.ReceivedAt` (when ingest accepted it), not the processing time, as the value written.
- Do not put the touch inside the feedback insert transaction.
- Duplicates (rows that already existed) still count as use; that is fine.

**JSON** (`internal/api/json.go`): `keyJSON` gains `LastUsedAt *string \`json:"lastUsedAt"\`` (RFC 3339 ms like the other times; null until first use) in every key response (list, create, revoke).

## 3. Channel last received

No new column. `store.Channel` gains `LastReceivedAt *int64`, filled by `ListChannels` (and `GetChannel` if the API returns it from there) with:
```sql
SELECT c.id, c.tenant_id, c.name, c.created_at,
       (SELECT MAX(f.received_at) FROM feedback f WHERE f.tenant_id = c.tenant_id AND f.channel_id = c.id)
FROM channels c WHERE c.tenant_id = ? ORDER BY c.created_at, c.id
```
The `(tenant_id, channel_id, received_at)` index serves the subquery. Check that index exists in the migrations; if it does not, add it in `0003` too and say so.
- `CreateChannel` returns `LastReceivedAt` nil.
- `channelJSON` gains `LastReceivedAt *string \`json:"lastReceivedAt"\`` (null when the channel has no feedback). The rename response, which re-lists, carries it.

## 4. Docs

- `docs/API.md`: Channel shape `{ "id", "name", "createdAt", "lastReceivedAt" }` with one sentence ("time of the newest feedback in the channel; null when it has none"). Key shape adds `lastUsedAt` ("when the key last sent feedback that was stored, updated at most once a minute; null until first use"). Export section: the filename rule from §1 with one example.
- `docs/ARCHITECTURE.md` processor section: one sentence that the processor also records ingest key last use (best effort, at most once per `KeyTouchInterval` per key, outside the insert transaction), so ingest stays write-free.

## 5. Tests (by name; all must exist and pass on SQLite and Postgres)

`internal/api` (pure function and handler)
- `TestExportFilenameAll` (no filters → `feedbaxter-all-<date>.csv`)
- `TestExportFilenameStatusAndTag` (`feedbaxter-new-bug-<date>.json`)
- `TestExportFilenameTagSanitised` ("Safari Export!" → `safari-export`; a tag of only symbols is left out; a 60-character tag is cut to 40)
- `TestExportContentDisposition` (the export handler sets the header with the computed name)
- `TestKeyJSONLastUsedAt` (null before, a time after `TouchIngestKeys`)
- `TestChannelJSONLastReceivedAt` (null for an empty channel; the newest `receivedAt` otherwise)

`internal/store`
- `TestMigration0003AddsLastUsedAt`
- `TestTouchIngestKeysSetsAndNeverGoesBack`
- `TestTouchIngestKeysIgnoresOtherTenantAndUnknownKey`
- `TestListChannelsLastReceivedAt`

`internal/processor`
- `TestProcessorTouchesKeyLastUsed` (a stored message sets `last_used_at` to its received time)
- `TestProcessorTouchesKeyAtMostOncePerInterval` (with a long interval, a second batch for the same key does not write; with `<= 0` it does)
- `TestProcessorTouchFailureDoesNotAffectFeedback` (make the touch fail, e.g. by dropping the `ingest_keys` table or the column in a test DB or swapping a failing querier: the feedback is still stored and acked)

`internal/ingest`
- No change; add `TestIngestDoesNotWriteDatabase` only if no equivalent test exists today (check `imports_test.go` first: it may already forbid the store import, which is enough).

## 6. Done means

`make vet && make test && make test-pg` green, and `cd web && npm test` still green (no front-end change expected). Report: files, Go test count before and after (`go test` summary lines or a count of `--- PASS`/`ok` packages), any existing test changed and why, deviations and why.
