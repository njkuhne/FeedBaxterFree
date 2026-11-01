# M1 brief: `internal/store` entity functions for the pipeline

Author: Opus director · Milestone M1 · Source: ARCHITECTURE §5, §7, §8; SPEC "Data model", "Ingest API"

Add the query functions the processor, retention, key sync and self-host bootstrap need. Builds on M0
(`store.Open`, `Dialect.Rebind/JSONType/Now`, `NewID`, `Querier`, `storetest.ForEachDialect`). No schema
change: `0001_core.sql` already has every column. No new dependency; go.mod is frozen.

**Files.** `internal/store/feedback.go`, `tenant.go`, `channel.go`, `ingestkey.go`, `errors.go`
(ErrNotFound); tests `feedback_test.go`, `tenant_test.go`, `ingestkey_test.go`. Touch nothing outside
`internal/store/` (storetest unchanged).

## Exported API

```go
package store

var ErrNotFound = errors.New("store: not found") // wraps sql.ErrNoRows cases

// ---- feedback.go
type FeedbackRow struct {
    ID, TenantID, ChannelID, Message string
    UserID, AppVersion, Platform    *string // nil = NULL
    Metadata                        []byte  // compact JSON object, nil = NULL
    IdempotencyKey                  string
    ReceivedAt                      int64   // Unix ms; updated_at is set to the same value
}
// InsertFeedbackBatch inserts rows with ON CONFLICT (tenant_id, idempotency_key) DO NOTHING and
// returns how many were actually inserted (sum of RowsAffected). status is left to its default 'new'.
// Postgres: multi-row VALUES, chunked so one statement never exceeds 1000 rows (11 params/row,
// well under the 65535-parameter limit); metadata bound as a string into the JSONB column.
// SQLite: one single-row statement, prepared once when q also implements
// PrepareContext (*sql.Tx, *sql.DB, *sql.Conn do), executed per row.
// Empty rows → (0, nil) without touching the DB. Caller owns the transaction.
func InsertFeedbackBatch(ctx context.Context, q Querier, d Dialect, rows []FeedbackRow) (int, error)

// DeleteFeedbackOlderThan deletes at most limit rows with received_at < cutoffMillis and returns the
// count. One statement valid on both dialects:
//   DELETE FROM feedback WHERE id IN (SELECT id FROM feedback WHERE received_at < ? ORDER BY received_at LIMIT ?)
func DeleteFeedbackOlderThan(ctx context.Context, q Querier, d Dialect, cutoffMillis int64, limit int) (int64, error)

// ---- tenant.go
type Tenant struct { ID, Name string; PlanID *string; BillingMonthStart int; CreatedAt int64 }
func CreateTenant(ctx context.Context, q Querier, d Dialect, name string, billingMonthStart int, now int64) (Tenant, error)
func GetTenant(ctx context.Context, q Querier, d Dialect, id string) (Tenant, error)     // ErrNotFound
func FirstTenant(ctx context.Context, q Querier, d Dialect) (Tenant, error)             // oldest by created_at, id; ErrNotFound when empty
func CountTenants(ctx context.Context, q Querier, d Dialect) (int, error)

// ---- channel.go
type Channel struct { ID, TenantID, Name string; CreatedAt int64 }
func CreateChannel(ctx context.Context, q Querier, d Dialect, tenantID, name string, now int64) (Channel, error)

// ---- ingestkey.go
// Ingest key format: "fbx_" + 32 lowercase base32 chars (a-z2-7) from crypto/rand (20 random bytes,
// unpadded StdEncoding, lower-cased). Display prefix = first 12 chars of the key ("fbx_" + 8).
// Stored hash = lowercase hex sha256 of the full key string. Only the hash and prefix are stored.
const IngestKeyPrefix = "fbx_"
func GenerateIngestKey() string
func HashIngestKey(key string) string  // hex(sha256(key))
func DisplayPrefix(key string) string  // key[:12]

type IngestKey struct { ID, TenantID, ChannelID, Prefix, Hash string; CreatedAt int64; RevokedAt *int64 }
// CreateIngestKey stores HashIngestKey(plaintext) and DisplayPrefix(plaintext); never the plaintext.
func CreateIngestKey(ctx context.Context, q Querier, d Dialect, tenantID, channelID, plaintext string, now int64) (IngestKey, error)
// RevokeIngestKey sets revoked_at = now if not already revoked; ErrNotFound if no such key.
func RevokeIngestKey(ctx context.Context, q Querier, d Dialect, id string, now int64) error

// CacheKey is one row as the ingest key cache needs it (ingest_keys JOIN tenants).
type CacheKey struct {
    KeyID, Hash, TenantID, ChannelID string
    Revoked           bool // revoked_at IS NOT NULL
    BillingMonthStart int  // tenants.billing_month_start
}
func ListIngestKeysForCache(ctx context.Context, q Querier, d Dialect) ([]CacheKey, error)               // every key, revoked included, ORDER BY id
func ListIngestKeysForCacheByTenant(ctx context.Context, q Querier, d Dialect, tenantID string) ([]CacheKey, error)
func GetIngestKeyForCache(ctx context.Context, q Querier, d Dialect, keyID string) (CacheKey, error)    // ErrNotFound
```

Rules: every query goes through `d.Rebind`; IDs from `NewID` with the matching prefix; timestamps are
the caller's `now` (Unix ms) so tests are deterministic; no `Now()` SQL needed. Errors wrap with
`fmt.Errorf("store: <op>: %w", err)`. `ErrNotFound` checked with `errors.Is`. All functions take the
Dialect explicitly (Querier may be a Tx, which does not know its dialect).

## Tests (every DB test via `storetest.ForEachDialect`, so it runs on SQLite and, under make test-pg, Postgres)

- `TestInsertFeedbackBatch` – 3 rows → 3 inserted; read back every column (NULLs for nil pointers and
  nil metadata; metadata JSON equal after unmarshal; status 'new'; updated_at == received_at).
- `TestInsertFeedbackBatchDedupes` – same batch again → 0 inserted, still 3 rows; a batch with one new
  and one duplicate idempotency key → 1; a batch containing the same key twice → 1; same key under
  another tenant → inserted.
- `TestInsertFeedbackBatchLarge` – 2500 rows in one call inside one tx → 2500 (exercises Postgres
  chunking); empty slice → 0, nil.
- `TestInsertFeedbackBatchInTxRollback` – insert in a tx, roll back → 0 rows (proves it honours the Querier).
- `TestDeleteFeedbackOlderThan` – rows at t-3, t-2, t-1, t+0 (ms); cutoff t-1 with limit 10 deletes 2
  and leaves the other 2; limit 1 on 5 old rows deletes exactly 1; nothing old → 0.
- `TestCreateTenantAndChannel` – CreateTenant then GetTenant round-trip (billing day 1, plan NULL);
  CountTenants 0 → 1; FirstTenant returns the older of two; GetTenant unknown → ErrNotFound; FirstTenant
  on empty → ErrNotFound; CreateChannel returns `ch_` ID; duplicate channel name in tenant errors.
- `TestGenerateIngestKey` (no DB) – 1000 keys: length 36, prefix `fbx_`, rest `[a-z2-7]{32}`, unique;
  HashIngestKey is 64 lowercase hex and equals sha256 computed in the test; DisplayPrefix length 12.
- `TestCreateIngestKeyStoresHashOnly` – row has hash == HashIngestKey(plain), prefix == DisplayPrefix;
  no column of the row contains the plaintext; `ky_` ID.
- `TestListIngestKeysForCache` – two tenants (billing days 1 and 15), three keys, one revoked: list
  returns all three with correct tenant/channel/hash/revoked/billing day; ByTenant filters; Get by ID
  matches; Get unknown → ErrNotFound; RevokeIngestKey twice keeps the first revoked_at; revoke unknown →
  ErrNotFound.

## Acceptance

1. `make vet`, `make test`, `make test-pg` green; paste `go test -v ./internal/store/... -run
   'Feedback|Tenant|IngestKey'` lines (from make test-pg) showing both sqlite and postgres subtests PASS.
2. No dialect branching except in InsertFeedbackBatch (multi-row vs prepared) — everything else is one
   SQL string through Rebind.
3. No plaintext ingest key is stored or logged anywhere in the package.

Non-negotiables that apply: processors are idempotent via the unique `(tenant_id, idempotency_key)`;
there is no client IP anywhere; timestamps are BIGINT Unix ms.
