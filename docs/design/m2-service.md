# M2 brief: `internal/service` (business logic and permission checks)

Author: Opus director · Milestone M2 · Source: ARCHITECTURE §9, §10; SPEC "Accounts, roles and auth", "MCP server", "Privacy"; m2-store.md

Transport-free business logic shared by the JSON API (M2) and the MCP server (M4). **Every function that
reads or writes tenant data takes an `ext.Principal`, uses only `p.TenantID` for scoping, and enforces the
role itself**, so a caller cannot forget a check. Touch `internal/service/` (new), and add the two small
exported helpers in `internal/ingest/counters.go` listed under Ops. First coder to need it: argon2 comes
from `golang.org/x/crypto` (already in go.sum as indirect at v0.58.0): import it, then run `go mod tidy`
in the container so it becomes a direct requirement. Do not `go get @latest`; nobody else touches go.mod.

## Types and errors (`service.go`, `errors.go`)

```go
type Service struct { DB *store.DB; Keys ext.KeyWriter; Queue *queue.Objects /* Ops; nil → error */
    Now func() time.Time /* nil = time.Now */; Log *slog.Logger; Version, Mode string /* Ops */ }
type Kind int
const ( KindInvalid Kind = iota + 1; KindUnauthenticated; KindForbidden; KindNotFound; KindConflict )
// Error: Kind + a plain sentence safe to show. Any other error is internal (logged, 500, no detail).
type Error struct { Kind Kind; Msg string }; func KindOf(err error) Kind // 0 when not an *Error
```
Map store.ErrNotFound/ErrConflict to KindNotFound/KindConflict with a sentence naming the thing. Never
put token, password, hash or SQL text in `Msg`.

`func requireAdmin(p ext.Principal) error` – the **only** role check; KindForbidden "Only admins can do
that." `func requireUser(p) error` – KindUnauthenticated when `p.UserID` or `p.TenantID` is empty, or
`p.Role` is not admin/reviewer. Every exported method below calls `requireUser` (or `requireAdmin`, which
calls it) first. Status changes need `p.Via` "ui"/"mcp".

## Auth (`auth.go`, `password.go`)

- `HashPassword(pw string) (string, error)`, `VerifyPassword(phc, pw string) bool`: argon2id t=1, m=64*1024
  KiB, p=4, key 32 bytes, 16-byte salt, PHC `$argon2id$v=19$m=65536,t=1,p=4$<b64 salt>$<b64 key>` (raw
  std base64); constant-time compare. **A semaphore of 2 bounds concurrent argon2 calls** (64 MiB each
  must not turn a login flood into an OOM).
- Passwords 10–256 characters. `GenerateTempPassword()`: 20 chars, crypto/rand, unambiguous alphabet
  (`abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789`), rejection sampling.
- `(s) Login(ctx, tenantID, email, password string) (store.User, error)`: email trimmed + lower-cased;
  unknown email still runs one VerifyPassword against a fixed dummy hash (timing); any failure →
  KindUnauthenticated "Email or password is incorrect."
- Sessions: token = 32 random bytes, base64url raw (43 chars); row id = hex sha256(token).
  `SessionTTL = 30*24h`, `sessionSlideEvery = time.Hour`.
  `CreateSession(ctx, u store.User) (token string, expires time.Time, error)` also calls
  `DeleteExpiredSessions` (opportunistic cleanup; its error is only logged).
  `ResolveSession(ctx, token) (ext.Principal, error)`: unknown → KindUnauthenticated; `expires_at <= now`
  → delete row, KindUnauthenticated; if `expires_at < now+TTL-slideEvery` set expiry to now+TTL (sliding,
  at most one write per hour per session). Returns `Principal{UserID, TenantID, Role, Via:"ui",
  MustChangePassword}`.
  `DeleteSession(ctx, token) error` (absent is fine).
- `ChangePassword(ctx, p, current, next, keepToken string) error`: verify current (KindInvalid "Current
  password is incorrect."), next must differ and pass the rules, set hash with must_change=false, delete
  the user's other sessions (keep the session of `keepToken`, hashed). Allowed while must-change is set.
- PATs: plaintext `fbx_pat_` + 32 lowercase base32 chars (20 random bytes); stored hash = hex sha256;
  `Prefix` = first 12 chars. `CreateToken(ctx, p, name) (store.Token, string, error)` (name trimmed 1–64);
  `ListTokens(ctx, p)`; `RevokeToken(ctx, p, id)` (own tokens only; another user's → KindNotFound). Any role.
  `PrincipalFromToken(ctx, token) (ext.Principal, error)`: malformed or unknown → KindUnauthenticated (no
  DB hit when malformed); `TouchToken` (error logged, not returned); `Via:"mcp"`, MustChangePassword false.

## Users (`users.go`)

`Me(ctx, p) (store.User, error)` any role. Admin only: `ListUsers`; `CreateUser(ctx, p, email, role)
(store.User, tempPassword string, error)` (email trimmed+lowered, contains one "@", ≤ 254; role admin|reviewer;
must_change=true; duplicate → KindConflict); `ResetPassword(ctx, p, userID) (string, error)` (not self →
KindInvalid "Use change password for your own account."; sets must_change and deletes the user's sessions,
not tokens); `SetRole(ctx, p, userID, role)`; `DeleteUser(ctx, p, userID)`.
Last-admin rule, in one transaction with `store.LockAdmins` first: demoting or deleting an admin when
`CountAdmins == 1` → KindConflict "An organisation needs at least one admin."; deleting yourself →
KindConflict "You cannot remove yourself." Deleting a user removes their sessions and tokens (FK cascade).

## Channels and keys (`channels.go`)

`ListChannels(ctx, p)` any role. Admin only: `CreateChannel(ctx, p, name)` (trimmed 1–64);
`RenameChannel(ctx, p, id, name)`; `ListKeys(ctx, p, channelID string)` ("" = all);
`CreateKey(ctx, p, channelID) (store.IngestKey, plaintext string, error)` – channel must exist in tenant;
`store.GenerateIngestKey`; after commit `Keys.SyncKey(keyID)`; a sync error is logged at warn and the key
is still returned (the reconcile repairs it within `KEY_CACHE_RESYNC_INTERVAL`).
`RevokeKey(ctx, p, keyID) (store.IngestKey, error)` – GetIngestKey (tenant check) → RevokeIngestKey →
`SyncKey`; **a sync error is returned** (internal) so the admin sees the failure and retries (revoke is
idempotent). `DeleteChannel(ctx, p, id)`: the tenant's only channel → KindConflict "Keep at least one
channel."; tx1 `RevokeChannelKeys` → commit → `SyncKey` each (so the cache says revoked even if the rest
fails; error returned); tx2 `DeleteChannel` (key rows cascade) → commit → `Keys.SyncTenant` (removes the
now orphaned entries; error logged). Feedback keeps its dangling `channel_id`; lists show a nil channel name.

## Feedback (`feedback.go`)

```go
type Filter struct { Statuses []string; ChannelID, Tag, UserID, Q string; From, To *time.Time }
type Page struct { Items []store.Feedback; NextCursor string } // "" = no more
type Detail struct { store.Feedback; History []store.StatusChange }
type BulkResult struct { Updated int; NotFound []string }   // NotFound never nil
const MaxBulkIDs = 500; const MaxLimit = 200; const DefaultLimit = 50
```
All any role unless marked. Validation → KindInvalid: unknown status; limit < 0 or > 200 (0 = default);
`From` after `To`; ids empty or > 500 (dedupe first, keep order); tag names trimmed 1–64, 1–20 per call;
`Q` ≤ 200 chars.
- `ListFeedback(ctx, p, f, cursor string, limit int) (Page, error)` – fetch `limit+1` to know whether a
  next page exists. Cursor = base64url(raw) of JSON `{"r":<received_at ms>,"i":"<id>"}`; undecodable →
  KindInvalid "Invalid cursor."
- `GetFeedback(ctx, p, id) (Detail, error)`.
- `SetStatus(ctx, p, ids []string, status string) (BulkResult, error)` – one tx, `store.SetFeedbackStatus`
  with `p.UserID`, `p.Via`. All four statuses allowed (`new` is the undo).
- `AddTags(ctx, p, ids, names) (BulkResult, error)` – one tx: EnsureTags + AddFeedbackTags. Updated =
  len(found ids). `RemoveTags` likewise.
- `ListTags(ctx, p) ([]store.Tag, error)`; `Stats(ctx, p, from, to *time.Time) (store.FeedbackStats, error)`.
- Admin: `DeleteFeedback(ctx, p, id) error`; `DeleteUserFeedback(ctx, p, userID string) (int64, error)`
  (userID non-empty). **Never touch counters**: deleted feedback still counts toward the month's limit.
- `Export(ctx, p, f Filter, fn func(store.Feedback) error) error` – loops ListFeedback pages of 500 with
  the store cursor and calls fn per row; memory is bounded by one page; stops on fn error or ctx.Done.

## Ops (`ops.go`)

```go
type Ops struct {
    Queue    struct { Depth uint64; OldestAgeSeconds float64; DLQ uint64 }
    Counters struct { Day, Month int64; DayKey, MonthKey string }
    DBSizeBytes int64
    Version, Mode, Dialect string
}
func (s *Service) Ops(ctx context.Context, p ext.Principal) (Ops, error) // admin
```
Depth/oldest from `Queue.Stream.Info(ctx)` (`State.Msgs`, `State.FirstTime`; age 0 when empty); DLQ from
`Queue.DLQ.Info` `State.Msgs`. Counters are read from the `counters` **KV bucket** (works in api mode where
no in-process Counters exist): add to `internal/ingest/counters.go` exported wrappers
`func DayKey(tenantID string, now time.Time) string` and `func MonthKey(tenantID string, billingDay int,
now time.Time) string` around the existing unexported ones; billing day from `store.GetTenant`; a missing
key reads as 0 (`jetstream.ErrKeyNotFound`). DB size from `store.DatabaseSize`.

## Tests (`*_test.go`, storetest.ForEachDialect; queuetest.Embedded + a real `keysync.Writer` where keys matter)

- `TestRoleMatrix` – **one table row per capability row of the spec** (view feedback/channels/tags/stats,
  status, tags, export, own PATs, delete feedback, delete by user, channel create/rename/delete, key
  list/create/revoke, user list/create/reset/role/delete, ops) × {admin, reviewer}: reviewer gets
  KindForbidden exactly on the admin-only rows and succeeds on the rest.
- `TestUnauthenticatedPrincipal` (every method), `TestTenantIsolation` (tenant B cannot touch A's rows).
- `TestPasswordHashRoundTrip`, `TestTempPassword`; `TestLoginAndSessions` (same error for wrong
  password/unknown email, expiry deletes, slide only after an hour with a fake Now, must-change carried);
  `TestChangePasswordKeepsCurrentSession`; `TestLastAdmin` (incl. self-delete); `TestResetPassword`
  (must_change, sessions gone, tokens kept, self → Invalid); `TestTokens` (format `^fbx_pat_[a-z2-7]{32}$`,
  Via "mcp", revoke, deleted user → unauthenticated, last_used touched).
- `TestKeyCreateReachesCache` (KV entry under the key's hash, revoked false → true after RevokeKey);
  `TestDeleteChannel` (KV revoked then removed, feedback kept, only channel → Conflict).
- `TestCursorPagination`, `TestListValidation`, `TestBulkLimits` (501 → Invalid, duplicates collapse,
  NotFound), `TestStatusHistoryVia`, `TestExportHonoursFilters` (1200 rows across pages, exact match and
  order), `TestDeleteDoesNotTouchCounters`, `TestOps` (2 unprocessed messages → depth 2 and age > 0, DLQ,
  counters from KV, DB size, reviewer forbidden).

## Acceptance

Tests green on both dialects; every exported method starts with requireUser/requireAdmin; no secrets logged.
