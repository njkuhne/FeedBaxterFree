# M0 brief: `internal/store` and `internal/store/storetest`

Author: Opus director · Milestone M0 · Source: ARCHITECTURE §8, SPEC "Data model", ADR 0004

Open SQLite or Postgres from config, apply embedded migrations recorded in `schema_migrations`, create
the full core schema, generate prefixed IDs, and give every later store/API test a one-line way to run
against both dialects. No entity query functions yet (M1 adds `feedback.go`, M2 the rest).

**Files.** `internal/store/`: `store.go` (DB, Open, Close, Querier), `dialect.go` (Dialect and its three methods,
nothing else), `migrate.go` (Migrate, loader), `migrations/0001_core.sql` (single file, see "JSON
column"), `id.go`, `storetest/storetest.go`; tests `dialect_test.go`, `id_test.go`, `store_test.go`,
`migrate_test.go`, `schema_test.go`.

## Exported API

```go
package store

type Dialect string
const ( SQLite Dialect = "sqlite"; Postgres Dialect = "postgres" )
func (d Dialect) Rebind(query string) string // Postgres: ? → $1,$2,… ; SQLite: unchanged
func (d Dialect) JSONType() string           // "TEXT" | "JSONB"
func (d Dialect) Now() string                // "CURRENT_TIMESTAMP" | "NOW()"

type DB struct { SQL *sql.DB; Dialect Dialect }
func Open(ctx context.Context, cfg config.Database) (*DB, error)
func (db *DB) Close() error

// Querier is satisfied by *sql.DB, *sql.Tx and *sql.Conn; entity functions take it.
type Querier interface { ExecContext(…) (sql.Result, error); QueryContext(…) (*sql.Rows, error); QueryRowContext(…) *sql.Row }

// Migrate applies pending core migrations, then pending extension migrations (sorted by
// Version), each in its own transaction together with its schema_migrations row.
// Returns the versions applied by this call, in order.
func Migrate(ctx context.Context, db *DB, extra []ext.Migration) ([]string, error)

type Prefix string
const (
    PrefixFeedback = "fb_"; PrefixChannel = "ch_"; PrefixUser = "us_"; PrefixTenant = "tn_"
    PrefixKey = "ky_"; PrefixToken = "tk_"; PrefixTag = "tg_"; PrefixStatusChange = "sc_" // all Prefix
)
func NewID(p Prefix) string // prefix + 20 lowercase base32 chars from crypto/rand
```

**Rebind**: rewrites `?` outside single-quoted string literals (with `''` escapes) and outside
double-quoted identifiers. It does not understand comments or Postgres `?` JSONB operators; doc-comment
that queries must not use them.

**NewID**: 13 bytes from `crypto/rand`, unpadded `base32.StdEncoding`, lower-cased, first 20 chars.

## Open

- SQLite: `os.MkdirAll(filepath.Dir(path), 0o750)`; DSN
  `file:<path>?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate`
  via driver name `sqlite` (`modernc.org/sqlite`). `_txlock=immediate` makes write transactions take the
  write lock at BEGIN, so concurrent writers wait on `busy_timeout` instead of failing on lock upgrade.
- Postgres: driver `pgx` from `github.com/jackc/pgx/v5/stdlib`, URL as given.
- Both: `PingContext`; on failure close and return an error naming the driver, never the URL. Unknown
  `Driver` → error. No pool tuning in M0.

## Migrations

- Files in `migrations/` embedded with `//go:embed migrations/*.sql`. Name forms:
  `NNNN_name.sql` (both dialects) or `NNNN_name.sqlite.sql` + `NNNN_name.postgres.sql` (both must exist).
  Version is `NNNN_name` (dialect suffix stripped). Loader fails on a malformed name or a half pair.
- `schema_migrations(version TEXT PRIMARY KEY, applied_at BIGINT NOT NULL /* Unix ms */)`, `CREATE TABLE IF NOT EXISTS` first.
- **JSON column**: migration SQL may contain the token `{{json}}`, replaced with `Dialect.JSONType()`
  before execution. This is how 0001 stays one file. No other tokens.
- Each migration: begin tx → exec the whole file (one `ExecContext`; verify both drivers accept
  multi-statement scripts and say so in your report; if one does not, split on `;` at end of line) →
  insert version row → commit.
- Postgres concurrency: hold `pg_advisory_lock(<fixed int64>)` on one `*sql.Conn` for the whole call
  (unlock in defer) and read applied versions after taking it. SQLite relies on `_txlock=immediate`.
- Extension migrations: `Version` must match `^ext_[0-9]{4}_[a-z0-9_]+$`, both bodies non-empty, no
  duplicate versions; else error before anything is applied. Core always runs first.

## Schema `0001_core.sql`

Conventions: `TEXT PRIMARY KEY` IDs; Unix-ms timestamps declared **`BIGINT`** (ARCHITECTURE says
"INTEGER"; Postgres `INTEGER` is 32-bit and overflows on Unix ms; SQLite gives `BIGINT` INTEGER affinity);
booleans `INTEGER NOT NULL DEFAULT 0 CHECK (x IN (0,1))` in both; `tenant_id TEXT NOT NULL` on every
table except `tenants`; plain `CREATE TABLE` (the migrator guarantees once-only).

| Table | Columns |
| --- | --- |
| `tenants` | id, name, plan_id (TEXT NULL, no FK: plans are SaaS-only, added by ext migration), billing_month_start (INTEGER NOT NULL DEFAULT 1, CHECK 1..28), created_at |
| `users` | id, tenant_id → tenants ON DELETE CASCADE, email, password_hash, role CHECK IN ('admin','reviewer'), must_change_password bool, created_at, updated_at; UNIQUE (tenant_id, email) |
| `sessions` | id (holds the hash of the cookie token; M2 decides the hash), tenant_id, user_id → users ON DELETE CASCADE, expires_at, created_at |
| `personal_access_tokens` | id, tenant_id, user_id → users ON DELETE CASCADE, name, hash UNIQUE, last_used_at NULL, created_at |
| `channels` | id, tenant_id → tenants ON DELETE CASCADE, name, created_at; UNIQUE (tenant_id, name) |
| `ingest_keys` | id, tenant_id, channel_id → channels ON DELETE CASCADE, prefix, hash UNIQUE, created_at, revoked_at NULL |
| `feedback` | id, tenant_id → tenants ON DELETE CASCADE, channel_id (**no FK**, see below), message NOT NULL, user_id NULL, metadata `{{json}}` NULL, app_version NULL, platform NULL, status NOT NULL DEFAULT 'new' CHECK IN ('new','reviewed','actioned','ignored'), idempotency_key NOT NULL, received_at, updated_at, updated_by NULL; UNIQUE (tenant_id, idempotency_key) |
| `tags` | id, tenant_id → tenants ON DELETE CASCADE, name, created_at; UNIQUE (tenant_id, name) |
| `feedback_tags` | tenant_id, feedback_id → feedback ON DELETE CASCADE, tag_id → tags ON DELETE CASCADE; PRIMARY KEY (feedback_id, tag_id) |
| `status_changes` | id (`sc_`), tenant_id, feedback_id → feedback ON DELETE CASCADE, from_status, to_status (same CHECK), user_id, via CHECK IN ('ui','mcp'), changed_at |

`feedback.channel_id` has no foreign key on purpose: a processor batch must never fail because a channel
was deleted between ingest and insert (that would push valid feedback into the DLQ). M2 decides what
channel deletion does to its feedback. Name the unique constraint on feedback
`feedback_tenant_idem_key` so M1's `ON CONFLICT (tenant_id, idempotency_key)` targets it by columns.

Indexes (named `<table>_<cols>_idx`): feedback (tenant_id, status, received_at DESC); feedback
(tenant_id, channel_id, received_at DESC); feedback (tenant_id, user_id); feedback (received_at) for the
retention sweep; feedback_tags (tag_id); status_changes (feedback_id, changed_at); sessions (user_id);
sessions (expires_at); ingest_keys (channel_id). Nothing else.

## `storetest`

```go
// ForEachDialect runs fn as subtest "sqlite" on a migrated SQLite DB in t.TempDir(), and as subtest
// "postgres" on a migrated, isolated Postgres schema when TEST_DATABASE_URL is set (else that subtest
// is skipped with t.Skip and a reason).
func ForEachDialect(t *testing.T, fn func(t *testing.T, db *store.DB))
// ForEachDialectEmpty is the same with no migrations applied (for migration tests).
func ForEachDialectEmpty(t *testing.T, fn func(t *testing.T, db *store.DB))
```
Postgres isolation: connect once with TEST_DATABASE_URL, `CREATE SCHEMA t_<random>`, then open the
`*store.DB` with `search_path=t_<random>` added to the URL query (pgx passes it as a runtime param);
`t.Cleanup` closes and runs `DROP SCHEMA … CASCADE`. Packages can then run in parallel against one
database. Both subtests close their DB in cleanup.

## Tests (all DB tests through `storetest`)

- `TestRebind` – no params; three params; `?` inside `'a?b'`, `'it''s ?'` and `"col?"` untouched;
  SQLite returns input unchanged. `TestJSONTypeAndNow` – the four values.
- `TestNewID` – every prefix: length `len(prefix)+20`, charset `[a-z2-7]`, 10 000 IDs unique.
- `TestOpenSQLitePragmas` – `PRAGMA journal_mode` = `wal`, `foreign_keys` = 1, `busy_timeout` = 5000,
  `synchronous` = 1; parent directory is created. `TestOpenUnknownDriver` – error.
- `TestMigrateFresh` (both) – returns `["0001_core"]`; every table in the schema table exists.
  `TestMigrateIdempotent` (both) – second call returns empty, no error, one row in schema_migrations.
- `TestMigrateExtension` (both) – an `ext_0001_demo` creating a table is applied after core, listed,
  and not re-applied; a bad version (`0002_x`) is rejected and nothing is applied.
- `TestMigrationFilenames` – the loader rejects a malformed name and an unpaired dialect file (use an
  `fstest.MapFS`; make the loader take an `fs.FS` internally for this).
- `TestIdempotencyKeyUnique` (both) – second plain INSERT with same (tenant, key) errors; the same row
  with `ON CONFLICT (tenant_id, idempotency_key) DO NOTHING` succeeds with 0 rows affected; same key
  under another tenant inserts.
- `TestTagAndChannelNamesUniquePerTenant` (both) – duplicate name rejected in one tenant, allowed in two.
- `TestTenantDeleteCascades` (both) – deleting a tenant removes its channels, users and feedback
  (proves `foreign_keys(ON)` is live on SQLite). `TestFeedbackChannelHasNoForeignKey` (both) – a
  nonexistent channel_id inserts. `TestTimestampsAreBigint` (both) – `received_at = 1<<40` round-trips.

## Acceptance

1. `make vet`, `make test` and `make test-pg` green; with `TEST_DATABASE_URL` set, the postgres subtests
   run (not skipped) — paste `go test -v ./internal/store/...` lines showing both subtests passing.
2. `dialect.go` has exactly the three methods; no other dialect branching outside `Open`, `Migrate` and
   `storetest`.
3. No dependency beyond modernc sqlite and pgx; no go.mod change. Coder touches only `internal/store/`.
