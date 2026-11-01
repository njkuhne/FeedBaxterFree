# M2 brief: `internal/store` for accounts, feedback triage and ops

Author: Opus director · Milestone M2 · Source: ARCHITECTURE §8, §9; SPEC "Data model", "Accounts, roles and auth", "Privacy"

Query functions the service layer (m2-service.md) needs. Same conventions as M1: hand-written SQL, `?`
placeholders through `d.Rebind`, every function `(ctx, q Querier, d Dialect, …)`, Unix-ms `int64` times,
errors wrapped `store: <op>: %w`. **Every query is tenant-scoped (`tenant_id = ?`) and fully
parameterised; no user input is ever concatenated into SQL** (only generated `?,?,?` lists). go.mod frozen
(pgconn is already part of pgx). Touch only `internal/store/`.

## Migration `0002_triage.sql` (one file, both dialects)

```sql
CREATE UNIQUE INDEX tags_tenant_id_lower_name_idx ON tags (tenant_id, lower(name));
CREATE INDEX feedback_tenant_id_received_at_id_idx ON feedback (tenant_id, received_at DESC, id DESC);
CREATE INDEX personal_access_tokens_user_id_idx ON personal_access_tokens (user_id);
ALTER TABLE personal_access_tokens ADD COLUMN prefix TEXT NOT NULL DEFAULT '';
```
Extend `schema_test.go` if it lists indexes/columns.

## Errors (`errors.go`)

`var ErrConflict = errors.New("store: conflict")`. Unexported `isUniqueViolation(err) bool`: pgx
`*pgconn.PgError` with Code `23505` (errors.As), or SQLite error text containing `UNIQUE constraint failed`.
Create/rename functions return `fmt.Errorf("store: …: %w", ErrConflict)` on a unique violation.

## Users (`user.go`)

```go
type User struct { ID, TenantID, Email, PasswordHash, Role string; MustChangePassword bool; CreatedAt, UpdatedAt int64 }
func CreateUser(ctx, q, d, u User) (User, error)          // assigns ID (us_) if empty; ErrConflict on (tenant_id,email)
func GetUser(ctx, q, d, tenantID, id string) (User, error)               // ErrNotFound
func GetUserByEmail(ctx, q, d, tenantID, email string) (User, error)     // exact match; the service lower-cases
func FirstAdmin(ctx, q, d, tenantID string) (User, error)               // oldest admin; ErrNotFound
func ListUsers(ctx, q, d, tenantID string) ([]User, error)              // created_at, id
func CountUsers(ctx, q, d, tenantID string) (int, error); func CountAdmins(ctx, q, d, tenantID string) (int, error)
// LockAdmins (in a tx): Postgres SELECT … role = 'admin' ORDER BY id FOR UPDATE; SQLite no-op (BEGIN IMMEDIATE).
func LockAdmins(ctx, q, d, tenantID string) error
func SetUserRole(ctx, q, d, tenantID, id, role string, now int64) error           // ErrNotFound
func SetUserPassword(ctx, q, d, tenantID, id, hash string, mustChange bool, now int64) error // ErrNotFound
func DeleteUser(ctx, q, d, tenantID, id string) error  // ErrNotFound; sessions and tokens go by FK cascade
```

## Sessions (`session.go`)

The row `id` is the lowercase hex sha256 of the cookie token; the token itself is never stored.
```go
type Session struct { ID, TenantID, UserID string; ExpiresAt, CreatedAt int64 }
func CreateSession(ctx, q, d, s Session) error; func SetSessionExpiry(ctx, q, d, id string, expiresAt int64) error
// GetSessionUser joins users; ErrNotFound when no row. No expiry check (the service does it and deletes).
func GetSessionUser(ctx, q, d, id string) (Session, User, error)
func DeleteSession(ctx, q, d, id string) error                        // no error when absent
func DeleteUserSessions(ctx, q, d, tenantID, userID, exceptID string) (int64, error) // exceptID "" = all
func DeleteExpiredSessions(ctx, q, d, now int64) (int64, error)
```

## Personal access tokens (`token.go`)

```go
type Token struct { ID, TenantID, UserID, Name, Prefix, Hash string; LastUsedAt *int64; CreatedAt int64 }
func CreateToken(ctx, q, d, t Token) (Token, error)          // assigns ID (tk_) if empty
func ListTokens(ctx, q, d, tenantID, userID string) ([]Token, error) // newest first; Hash not needed by callers but may be filled
func DeleteToken(ctx, q, d, tenantID, userID, id string) error       // ErrNotFound unless the row belongs to that user
func GetTokenUser(ctx, q, d, hash string) (Token, User, error)       // ErrNotFound
func TouchToken(ctx, q, d, id string, now int64) error // only when NULL or older than now-60000
```

## Channels and keys (`channel.go`, `ingestkey.go`)

`CreateChannel` maps a unique violation to ErrConflict. Add:
```go
func GetChannel(ctx, q, d, tenantID, id string) (Channel, error); func CountChannels(…) (int, error)
func ListChannels(ctx, q, d, tenantID string) ([]Channel, error)            // created_at, id
func RenameChannel(ctx, q, d, tenantID, id, name string) error              // ErrNotFound, ErrConflict
func DeleteChannel(ctx, q, d, tenantID, id string) error                    // ErrNotFound; key rows cascade; feedback untouched
func ListIngestKeys(ctx, q, d, tenantID, channelID string) ([]IngestKey, error) // channelID "" = all; created_at DESC, id
func GetIngestKey(ctx, q, d, tenantID, id string) (IngestKey, error)          // ErrNotFound
func RevokeChannelKeys(ctx, q, d, tenantID, channelID string, now int64) ([]string, error) // returns all its key IDs
```

## Feedback (`feedback.go`, new `feedback_query.go`)

```go
type Feedback struct { ID, TenantID, ChannelID string; ChannelName *string /* nil: channel deleted (LEFT JOIN) */
    Message string; UserID, AppVersion, Platform *string; Metadata json.RawMessage /* nil when NULL */
    Status string; ReceivedAt, UpdatedAt int64; UpdatedBy *string; Tags []string /* lower(name) order, never nil */ }
type FeedbackFilter struct { Statuses []string /* OR'd */; ChannelID, Tag /* name, case-insensitive */, UserID /* exact */ string
    From, To *int64 /* received_at >= From, < To */; Q string /* case-insensitive substring of message */ }
type Cursor struct { ReceivedAt int64; ID string }   // the last row of the previous page
func ListFeedback(ctx, q, d, tenantID string, f FeedbackFilter, after *Cursor, limit int) ([]Feedback, error)
func GetFeedback(ctx, q, d, tenantID, id string) (Feedback, error)   // with tags; ErrNotFound
```
- One unexported builder `feedbackWhere(tenantID string, f FeedbackFilter) (string, []any)` used by
  ListFeedback and FeedbackStats. Tag: `EXISTS (SELECT 1 FROM feedback_tags ft JOIN tags t ON t.id =
  ft.tag_id WHERE ft.feedback_id = f.id AND t.tenant_id = ? AND lower(t.name) = lower(?))`.
  Q: `lower(f.message) LIKE lower(?) ESCAPE '!'` with the argument `"%" + escapeLike(q) + "%"`, where
  `escapeLike` prefixes each of `!`, `%`, `_` with `!`. (SQLite `lower` folds ASCII only; acceptable.)
- Cursor: `AND (f.received_at < ? OR (f.received_at = ? AND f.id < ?))`; `ORDER BY f.received_at DESC,
  f.id DESC LIMIT ?`.
- **Tags in one extra query per page, never per row**: `SELECT ft.feedback_id, t.name FROM feedback_tags
  ft JOIN tags t ON t.id = ft.tag_id WHERE ft.feedback_id IN (?,…) ORDER BY lower(t.name)`. Unexported
  `attachTags` shared by List and Get.

```go
type StatusChange struct { ID, FeedbackID, From, To, UserID string; UserEmail *string; Via string; ChangedAt int64 }
func ListStatusChanges(ctx, q, d, tenantID, feedbackID string) ([]StatusChange, error) // changed_at, id; LEFT JOIN users for email
// SetFeedbackStatus (caller's tx): one IN read of current statuses, one UPDATE (status, updated_at/by) of
// rows that differ, one status_changes row (sc_) per changed row. Returns changed and missing IDs.
func SetFeedbackStatus(ctx, q, d, tenantID string, ids []string, status, userID, via string, now int64) (changed, missing []string, err error)
// EnsureTags creates missing tags (tg_); case-insensitive match keeps the stored spelling; duplicates collapse.
func EnsureTags(ctx, q, d, tenantID string, names []string, now int64) ([]Tag, error)
type Tag struct { ID, Name string; Count int64 }
// AddFeedbackTags: per tag INSERT … SELECT ?, f.id, ? FROM feedback f WHERE tenant AND id IN (…) ON CONFLICT DO NOTHING.
func AddFeedbackTags(ctx, q, d, tenantID string, ids, tagIDs []string) (missing []string, err error)
func RemoveFeedbackTags(ctx, q, d, tenantID string, ids, names []string) (missing []string, err error) // names case-insensitive; tags stay
func ListTags(ctx, q, d, tenantID string) ([]Tag, error)  // with link counts (0 allowed), lower(name)
type FeedbackStats struct { Total int64; ByStatus map[string]int64; ByChannel []ChannelCount /* {ChannelID; Name *string; Count} */; ByTag []Tag }
func GetFeedbackStats(ctx, q, d, tenantID string, from, to *int64) (FeedbackStats, error) // 3 GROUP BYs; ByStatus has all 4 keys
func DeleteFeedback(ctx, q, d, tenantID, id string) error  // ErrNotFound; tags/history cascade. Also DeleteFeedbackByUser(…, userID) (int64, error)
```
"Missing" = IDs not found in the tenant (one `SELECT id … IN` query). Callers pass ≤ 500 deduplicated IDs.
Delete never touches counters (that is KV, not the store) – deleted feedback still counts toward quota.

## Ops (`store.go`)

`func DatabaseSize(ctx, db *DB) (int64, error)`: SQLite `SELECT page_count * page_size FROM
pragma_page_count(), pragma_page_size()`; Postgres `SELECT pg_database_size(current_database())`.

## Tests (all `storetest.ForEachDialect`; seed helpers in a `_test.go` file)

`TestUserCRUD` (create/get/by email, duplicate → ErrConflict, role/password updates, FirstAdmin,
CountAdmins); `TestDeleteUserCascades` (sessions, tokens go); `TestSessionLifecycle` (join returns
role/must-change, expiry update, delete, DeleteUserSessions with/without exceptID, expired cleanup);
`TestTokenLifecycle` (newest first, GetTokenUser, other user's delete → ErrNotFound, TouchToken once per
60 s); `TestChannelCRUD` (rename conflict, duplicate create, delete keeps feedback with nil ChannelName,
cross-tenant → ErrNotFound); `TestRevokeChannelKeys`, `TestListIngestKeys`; `TestListFeedbackFilters`
(table over each filter alone and combined, tag in another case, from inclusive/to exclusive, tenant
isolation); `TestListFeedbackLikeEscaping` (`50%`, `a_b`, `!` literal); `TestListFeedbackCursor` (7 rows,
two sharing received_at, pages of 3 visit each once in order); `TestListFeedbackTagsOneQuery`;
`TestSetFeedbackStatus` (changed/unchanged/missing, history only for changes, updated_by);
`TestEnsureTagsCaseInsensitive` (+ the unique index rejects a raw duplicate); `TestAddRemoveTags`
(idempotent, other-case remove, missing IDs, ListTags counts); `TestFeedbackStats` (all 4 status keys,
deleted channel → nil name, tags, range); `TestDeleteFeedback` (cascade, by user count, isolation);
`TestDatabaseSize`.

## Acceptance

`make vet`, `make test`, `make test-pg` green (paste the tails). No string concatenation of input into
SQL. No N+1. Existing M1 tests untouched and passing.
