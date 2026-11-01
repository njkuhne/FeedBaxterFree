# M2 brief: `internal/api`, self-host auth, admin bootstrap, wiring

Author: Opus director · Milestone M2 · Source: ARCHITECTURE §3, §9, §11; SPEC "Accounts, roles and auth"; m2-service.md

Thin HTTP over `internal/service`, which enforces every role rule. Files: `internal/api/*.go` + tests;
`internal/selfhost/{auth.go,bootstrap.go}` (move Bootstrap) + tests; `ext/ext.go`; `internal/config`;
`feedbaxter.go` (+ test); `cmd/feedbaxter/main.go`; `.env.example`. go.mod frozen.

## ext changes (standard library only) — decision record

```go
type Settings struct { HTTPAddr, DataDir, Mode string }      // Deps.Settings, filled from config
// SessionStore issues and resolves fb_session cookies. One implementation (internal/api) owns the cookie
// rules for every Authenticator: random 32-byte token, sha256 stored, HttpOnly, SameSite=Lax, Secure when
// r.TLS != nil or X-Forwarded-Proto is https, Path=/, 30-day sliding expiry.
type SessionStore interface {
    Start(w http.ResponseWriter, r *http.Request, userID, tenantID string) error
    Principal(r *http.Request) (Principal, bool)   // Via "ui"
    End(w http.ResponseWriter, r *http.Request) error
}
func WithPrincipal(ctx context.Context, p Principal) context.Context
func PrincipalFrom(ctx context.Context) (Principal, bool)
```
`Deps` gains `Settings Settings` and `Sessions SessionStore` (nil outside api/all). `Authenticator`
keeps its two methods; its doc now says: Mount registers on the mux that serves `/api/`; every request
there has passed the CSRF check and carries the resolved principal (if any) via `PrincipalFrom`.
**No Service in Deps**: the service exposes internal types. The self-host Authenticator is built inside
feedbaxter.go with internal types; a SaaS Authenticator uses `Deps.Sessions` + `Deps.DB` (open issue: it
must hash passwords itself, x/crypto argon2id with the same PHC format).
`Start` takes userID and tenantID (it calls `service.CreateSession(store.User{ID, TenantID})`), so a SaaS
Authenticator never needs internal types.

## internal/api

```go
type Config struct { Svc *service.Service; Auth ext.Authenticator; Deps ext.Deps; Log *slog.Logger }
func NewSessionStore(svc *service.Service) ext.SessionStore
func New(cfg Config) http.Handler   // serves /api/…; feedbaxter.go mounts it at "/api/"
```
`New` builds an inner `http.ServeMux`, calls `cfg.Auth.Mount(inner, cfg.Deps)`, registers the routes
below, and wraps it: (1) **CSRF**: for any method other than GET/HEAD/OPTIONS require `Sec-Fetch-Site`
∈ {same-origin, none} or a non-empty `X-Requested-With`, else 403 `csrf`. (2) `p, ok := Auth.Principal(r)`;
if ok, `ext.WithPrincipal`. (3) if `ok && p.MustChangePassword` and the route is not `POST
/api/auth/password`, `/logout`, `/login` or `GET /api/me` → 403 `password_change_required`. (4) unknown
`/api/` path → 404 `not_found` JSON. Handlers take the principal with a helper that returns 401
`unauthenticated` when absent. **PATs are never accepted here** (cookie only). Set `Cache-Control:
no-store` on every /api response.

JSON: request bodies `http.MaxBytesReader` 1 MiB (413 `payload_too_large`), `DisallowUnknownFields`,
one value only; malformed → 400 `invalid_request`. Times out as RFC 3339 UTC with milliseconds
(`2006-01-02T15:04:05.000Z`), in as RFC 3339. camelCase field names. `null` for absent optional values.
Errors (errors.go): `{"error":code,"message":sentence}`; service Kind → 400 `invalid_request`, 401
`unauthenticated`, 403 `forbidden`, 404 `not_found`, 409 `conflict`; anything else → 500 `internal`
"Something went wrong." with the error logged at error (never the request body).

| Method path | Role | Body → response |
| --- | --- | --- |
| GET /api/me | any (also while must-change) | → `{id,email,role,mustChangePassword}` |
| GET /api/users | admin | → `{items:[user]}` user = `{id,email,role,mustChangePassword,createdAt}` |
| PATCH /api/users/{id} | admin | `{role}` → user |
| DELETE /api/users/{id} | admin | → 204 |
| GET/POST /api/tokens | any, own | POST `{name}` → 201 `{token:{id,name,prefix,createdAt,lastUsedAt},secret}`; GET → `{items:[token]}` |
| DELETE /api/tokens/{id} | any, own | → 204 |
| GET/POST /api/channels | GET any, POST admin | POST `{name}` → 201 channel `{id,name,createdAt}` |
| PATCH/DELETE /api/channels/{id} | admin | PATCH `{name}` → channel; DELETE → 204 |
| GET /api/keys?channel= | admin | → `{items:[{id,channelId,prefix,createdAt,revokedAt}]}` |
| POST /api/channels/{id}/keys | admin | → 201 `{key:{…},secret}` |
| POST /api/keys/{id}/revoke | admin | → key |
| GET /api/feedback | any | filters below → `{items:[feedback],nextCursor}` |
| GET /api/feedback/{id} | any | → feedback + `history:[{from,to,userId,userEmail,via,at}]` |
| PATCH /api/feedback/{id} | any | `{status}` → feedback (404 when missing) |
| DELETE /api/feedback/{id} | admin | → 204 |
| POST /api/feedback/status | any | `{ids,status}` → `{updated,notFound}` |
| POST /api/feedback/add-tags, /remove-tags | any | `{ids,tags}` → `{updated,notFound}` |
| POST /api/feedback/delete-by-user | admin | `{userId}` → `{deleted}` |
| GET /api/feedback/export?format=csv\|json | any | filters below, streamed |
| GET /api/tags | any | → `{items:[{id,name,count}]}` |
| GET /api/stats?from&to | any | → `{total,byStatus,byChannel:[{channelId,name,count}],byTag:[{name,count}]}` |
| GET /api/ops | admin | → `{queue:{depth,oldestAgeSeconds,dlq},counters:{day,month,dayKey,monthKey},dbSizeBytes,version,mode,dialect}` |

feedback = `{id,channelId,channelName,message,userId,appVersion,platform,metadata,status,receivedAt,
updatedAt,updatedBy,tags}`; `channelName` null for a deleted channel; `metadata` raw object or null.
Filters (list and export): `status` repeatable and/or comma-separated, `channel`, `tag`, `userId`, `from`,
`to` (RFC 3339), `q`, plus `cursor`, `limit` on list. Bad values → 400.

**Export** (export.go): validate everything (format, filters) before writing; then headers
`Content-Type: text/csv; charset=utf-8` or `application/json`, `Content-Disposition: attachment;
filename="feedback-YYYYMMDD.csv|json"`, stream via `svc.Export` with `encoding/csv` (`UseCRLF = true`, no
BOM) or a hand-written JSON array (`[`, items joined by `,`, `]`, each `json.Marshal`ed), flushing via
`http.ResponseController` every page. CSV header: `id,received_at,status,channel,user_id,platform,
app_version,tags,message,metadata`; tags joined with `; `; channel is the name or empty. Spreadsheet
formula guard: a text cell starting with `=`, `+`, `-`, `@`, tab or CR gets a leading `'` (CSV only;
JSON is verbatim). The server's WriteTimeout (30 s) would cut long exports: extend the deadline per page
with `ResponseController.SetWriteDeadline(now+30s)`. A mid-stream error is logged; the response just ends.

## internal/selfhost

`auth.go`: `type Auth struct{ Svc *service.Service; Sessions ext.SessionStore; Tenants ext.TenantResolver }`
implements ext.Authenticator. `Principal(r)` = `Sessions.Principal(r)`. Mount routes:
`POST /api/auth/login {email,password}` → tenant = `Tenants.Default`, `Svc.Login`, `Sessions.Start` →
200 `{user}` (same shape as /api/me); `POST /api/auth/logout` → `Sessions.End` → 204 (also without a
session); `POST /api/auth/password {currentPassword,newPassword}` → `Svc.ChangePassword` keeping the
current session → 204; `POST /api/users {email,role}` (admin via service) → 201
`{user,temporaryPassword}`; `POST /api/users/{id}/reset-password` → `{temporaryPassword}`. Selfhost may
import `internal/api` only for the shared JSON/error helpers if you export them there as `api.WriteJSON`,
`api.WriteError(w, log, err)`, `api.DecodeJSON` — do so rather than duplicating.

`bootstrap.go`: `Bootstrap(ctx, db, keys, log, now, adminEmail string) (created bool, err error)`.
Tenant creation as in M1; **independently**, when the default tenant has no users (also true for an M1
database upgraded in place) create the admin (`adminEmail` lower-cased, role admin, must_change) with
`service.GenerateTempPassword`, and after commit log once at info exactly
`admin login <email> temporary password: <pw> (you must change it at first login)`. `created` = anything
created. `ResetAdmin(ctx, db, tenantID, email string, now) (gotEmail, pw string, err)`: email given → that
user, must be an admin (else error "user is not an admin"); empty → `store.FirstAdmin`; no users at all →
create the admin with `email` or `admin@localhost`. Sets a new temp password, must_change, deletes its
sessions.

## config, wiring, CLI

`ADMIN_EMAIL` (default `admin@localhost`) in config, `.env.example`, tests. feedbaxter.go in
api/all: `svc := &service.Service{DB, Keys: kw, Queue: objs, Log, Version, Mode}`; `sessions :=
api.NewSessionStore(svc)`; nil `opts.Authenticator` → `&selfhost.Auth{…}`; `deps.Settings`,
`deps.Sessions`; `mux.Handle("/api/", api.New(…))` replaces the direct `Authenticator.Mount` call
(MountRoutes stays on the outer mux). Bootstrap gets `cfg.AdminEmail`. Export
`func ResetAdmin(ctx context.Context, opts Options, email string) (string, string, error)`: config, open,
migrate, tenant via `opts.TenantResolver` or `store.FirstTenant`, `selfhost.ResetAdmin`. main.go:
`reset-admin [--email=]` prints `temporary password for <email>: <pw>` to stdout; update usage + test.

## Tests

`internal/api/api_test.go` (httptest over `api.New` + selfhost.Auth, storetest.ForEachDialect, queuetest,
real keysync.Writer): `TestLoginFlowForcedChange` (login → cookie flags HttpOnly/SameSite=Lax/no Secure;
/api/feedback → 403 password_change_required; /api/me ok; change → feedback 200; wrong password 401);
`TestSecureCookieBehindTLSProxy`; `TestCSRF` (POST without Sec-Fetch-Site/X-Requested-With → 403 csrf;
cross-site → 403; same-origin ok); `TestLogoutEndsSession`; `TestPATNotAcceptedOnAPI`; `TestEndpointSmoke`
(every route above once as admin: status + shape); `TestReviewerForbidden` (every admin route → 403
forbidden as reviewer); `TestFeedbackPagination` (limit 2 over 5 rows via nextCursor);
`TestListFilters` (status repeated + comma, q, tag, from/to); `TestExportCSVQuoting` (comma, quote,
newline, `=cmd` guard, CRLF, filter honoured); `TestExportJSON` (valid array, filter honoured);
`TestOpsShape`; `TestErrorShape` (404 unknown path, 400 unknown field, 413). selfhost:
`TestBootstrapCreatesAdmin` (log regex `admin login admin@localhost temporary password: [A-Za-z2-9]{20} `;
login works with it; second call logs nothing), `TestBootstrapUpgradesM1Database`, `TestResetAdmin`.
`feedbaxter_test.go` `TestRunAllAdminRoundTrip`: boot all mode, take the temp password from the log,
login, change password, create channel + key via API, POST /v1/feedback with it → 202, poll
/api/feedback until it shows the message, revoke the key, poll POST until 401 (deadline 5 s).

## Acceptance

Tests green on both dialects; `make dev` round trip works; secrets only in their one response/log line.
