# M1 brief: `internal/ingest/keysync` and `ext.KeyWriter`

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §3, §5; ADR 0003. Companion to `m1-ingest.md`.

The DB → KV side of the key cache: write an entry from a store row plus limits, and a periodic reconcile
(every `KEY_CACHE_RESYNC_INTERVAL`, api/all mode). Separate package so `internal/ingest` stays DB-free.

**Files.** `internal/ingest/keysync/keysync.go`, `keysync_test.go`; `ext/ext.go` gains `KeyWriter` only.

## API

```go
// in ext/ext.go
// KeyWriter rewrites ingest key cache entries from the database. The api mode calls it after every
// key, channel, tenant or limit change; a periodic reconcile repairs anything missed.
type KeyWriter interface {
    SyncKey(ctx context.Context, keyID string) error       // upsert that key's entry (revoked included)
    SyncTenant(ctx context.Context, tenantID string) error // upsert the tenant's keys; delete its orphans
}

package keysync
type Writer struct { DB *store.DB; KV jetstream.KeyValue; Limits ext.LimitsSource; Log *slog.Logger }
func EntryFor(k store.CacheKey, lim ext.Limits) ingest.Entry
func (w *Writer) SyncKey(ctx context.Context, keyID string) error   // ErrNotFound row → returns the error
func (w *Writer) SyncTenant(ctx context.Context, tenantID string) error
type Stats struct { Put, Deleted int }
// Reconcile: all store.ListIngestKeysForCache rows → desired entries (limits fetched once per tenant);
// for each, Get and Put only if the bytes differ; then ListKeys and Delete every key not desired.
func (w *Writer) Reconcile(ctx context.Context) (Stats, error)
// Run reconciles once immediately, then every interval, until ctx is done; errors are logged.
func (w *Writer) Run(ctx context.Context, interval time.Duration)
```
`var _ ext.KeyWriter = (*Writer)(nil)`. Puts use `kv.Put` (keys bucket has no TTL).

## Tests (storetest.ForEachDialect + queuetest.Embedded; run under make test and make test-pg)

`TestHashMatchesStore`; `TestSyncKey` (create →
entry JSON exact incl. billing_month_start and limits from a fake LimitsSource; revoke → revoked true);
`TestReconcile` (stale value fixed, missing entry added, orphan KV key deleted, second run Stats{0,0});
`TestSyncTenantDeletesOrphans`; `TestCacheSeesWriterChanges` (KeyCache.Run + Writer: create/revoke/
limit change visible to Lookup within 5s).

## Acceptance

`make vet`, `make test`, `make test-pg` green with keysync tests passing on both dialects. Reconcile is
idempotent (second run writes nothing). `ext` still imports only the standard library.
