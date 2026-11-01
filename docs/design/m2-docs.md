# M2 brief: `docs/API.md` and the `docs/OPS.md` review

Author: Opus director · Milestone M2 · Source: m2-api.md; the code in `internal/api`, `internal/selfhost/auth.go`, `internal/service`

M3's UI coders build the admin UI from `docs/API.md` alone, so it must match the code exactly. Write it
from the code, not from the brief; where they differ, the code wins and the difference goes in the report.

## docs/API.md (new)

1. **Conventions**: base path `/api`; JSON in and out (camelCase); times RFC 3339 UTC with milliseconds;
   absent optional values are `null`; bodies ≤ 1 MiB, unknown fields rejected; `Cache-Control: no-store`.
2. **Authentication**: `fb_session` cookie (HttpOnly, SameSite=Lax, Secure behind TLS or
   `X-Forwarded-Proto: https`, 30-day sliding expiry; `GET /api/me` renews the cookie). CSRF rule for
   every non-GET/HEAD/OPTIONS request (`Sec-Fetch-Site` same-origin/none, or an `X-Requested-With` header;
   the UI should always send `X-Requested-With: fetch`). Must-change-password gate and the three routes it
   allows. Personal access tokens are for `/mcp` (M4) and are not accepted on `/api`.
3. **Roles**: the spec's capability table, and which endpoints are admin-only.
4. **Errors**: the body shape and every code with its status (`invalid_request` 400, `unauthenticated`
   401, `forbidden`/`password_change_required`/`csrf` 403, `not_found` 404, `method_not_allowed` 405,
   `conflict` 409, `payload_too_large` 413, `internal` 500), plus the notable 409s (last admin, removing
   yourself, last channel, duplicate names/emails).
5. **Endpoints**, grouped (auth, me, users, tokens, channels, keys, feedback, tags, stats, export, ops):
   for each, method + path, role, request body or query, success status, response shape with a short
   example, and endpoint-specific errors. Mark self-host-only routes (login, logout, password, create
   user, reset password: provided by the self-host Authenticator; SaaS supplies its own).
6. **Feedback filters**: `status` (repeatable and/or comma-separated; new, reviewed, actioned, ignored),
   `channel`, `tag` (case-insensitive name), `userId` (exact), `from` (inclusive) / `to` (exclusive),
   `q` (case-insensitive substring, ≤ 200 chars); list adds `limit` (1–200, default 50) and `cursor`.
7. **Pagination**: opaque `nextCursor`, `null` on the last page; order received time desc then id desc;
   pass back unchanged with the same filters.
8. **Bulk**: ≤ 500 IDs, duplicates ignored, `{updated, notFound}`; status history `via`.
9. **Export**: formats, filename, CSV columns, CRLF, RFC 4180 quoting, tags joined `; `, the
   spreadsheet formula guard, JSON as an array; a failure mid-stream aborts the connection.
10. **Semantics worth knowing for the UI**: secrets (key, token, temporary password) are returned once;
    deleting a channel revokes its keys and keeps its feedback (`channelName: null`); deleted feedback
    still counts toward the month's limit; feedback text is plain text (preserve line breaks, never
    render as HTML or Markdown); `userId` is opaque.

## docs/OPS.md (review pass)

First boot now also logs `admin login <email> temporary password: … (you must change it at first
login)`; `ADMIN_EMAIL` in the configuration table; `reset-admin [--email]` usage inside the container
(`docker compose exec feedbaxter /feedbaxter reset-admin`); replace the "will be shown on /api/ops (M2)"
note with what `/api/ops` returns; key management now exists (create/revoke via the API; the bootstrap
key can be revoked). Keep everything else as is.

## Acceptance

Every route registered in `internal/api/api.go` and `internal/selfhost/auth.go` appears in API.md with
the right method, path and role, and every JSON field name matches the structs in `internal/api/json.go`.
No code changes.
