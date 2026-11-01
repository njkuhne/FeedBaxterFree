# Admin API

The JSON API behind the admin UI. Ingest (`POST /v1/feedback`) is a separate API and is not described here. Source of truth: `internal/api`, `internal/selfhost/auth.go` and `internal/service`.

## Conventions

- Base path `/api`. Requests and responses are JSON with camelCase field names. Request bodies are at most 1 MiB, must be a single JSON value, and unknown fields are rejected (`400 invalid_request`; over the limit is `413 payload_too_large`).
- Times are RFC 3339 UTC with milliseconds, for example `2026-03-01T09:30:00.000Z`. Query parameters that take a time (`from`, `to`) accept any RFC 3339 value.
- Absent optional values are `null`, never omitted. Lists that are empty are `[]`.
- IDs are opaque strings (`fb_…` feedback, `ch_…` channel, `us_…` user, `ky_…` key, `tk_…` token, `tg_…` tag). Do not parse them.
- Every response carries `Cache-Control: no-store`.
- `204` responses have no body. All other success and error responses are `application/json`, except the export.
- Unknown paths answer `404 not_found`; a known path with the wrong method answers `405 method_not_allowed` with an `Allow` header. Both use the error body below, and both can be returned before authentication is checked.

## Authentication

Sign in with `POST /api/auth/login`. The response sets the `fb_session` cookie: HttpOnly, SameSite=Lax, path `/`, 30-day sliding expiry, `Secure` when the request came over TLS or with `X-Forwarded-Proto: https`. The server-side expiry slides on use; `GET /api/me` also re-issues the cookie with a fresh 30 days, so the UI should call it on every page load. An expired or unknown session is `401 unauthenticated`; send the user to the sign-in screen.

Personal access tokens (`fbx_pat_…`) are for the MCP server at `/mcp` and are never accepted on `/api`. See [MCP.md](MCP.md).

**CSRF.** Every request except GET, HEAD and OPTIONS must either carry `Sec-Fetch-Site: same-origin` (or `none`), or a non-empty `X-Requested-With` header. Otherwise the answer is `403 csrf`. The UI should always send `X-Requested-With: fetch`. This applies to login too.

**Must-change-password gate.** A user whose `mustChangePassword` is true (first admin login, new user, after a reset) gets `403 password_change_required` on every request except:

| Method and path | Why |
| --- | --- |
| `GET /api/me` | learn the state |
| `POST /api/auth/password` | change it |
| `POST /api/auth/logout` | leave |
| `POST /api/auth/login` | sign in again or as someone else (replaces the session) |

## Roles

Two roles: `admin` and `reviewer`. The server enforces them; a reviewer calling an admin endpoint gets `403 forbidden` ("Only admins can do that."). Hide admin controls from reviewers, but never rely on that.

| Capability | Admin | Reviewer | Endpoints |
| --- | --- | --- | --- |
| View feedback, tags, stats; export | yes | yes | `GET /api/feedback…`, `/api/tags`, `/api/stats` |
| Set status, add and remove tags | yes | yes | `PATCH /api/feedback/{id}`, `POST /api/feedback/status`, `add-tags`, `remove-tags` |
| List channels | yes | yes | `GET /api/channels` |
| Own profile, password, own tokens | yes | yes | `/api/me`, `/api/auth/*`, `/api/tokens…` |
| Delete feedback | yes | no | `DELETE /api/feedback/{id}`, `POST /api/feedback/delete-by-user` |
| Channels (create, rename, delete) and keys | yes | no | `/api/channels`, `/api/keys…` |
| People | yes | no | `/api/users…` |
| Operations | yes | no | `GET /api/ops` |

Billing and deleting the org are not part of the self-host API.

## Errors

Every error has the same body:

```json
{ "error": "invalid_request", "message": "Enter a valid email address." }
```

`message` is plain text suitable for showing to the user; branch on `error` and the status.

| Status | `error` | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | Bad JSON, unknown field, or a failed validation (message says which). |
| 401 | `unauthenticated` | No valid session, or wrong email or password at login. |
| 403 | `forbidden` | The role may not do this. |
| 403 | `password_change_required` | See the gate above. |
| 403 | `csrf` | Missing same-origin proof. |
| 404 | `not_found` | Unknown route, or the item does not exist (or belongs to another org). |
| 405 | `method_not_allowed` | Wrong method for the path. |
| 409 | `conflict` | The change would break a rule (list below). |
| 413 | `payload_too_large` | Body over 1 MiB. |
| 500 | `internal` | Message is always "Something went wrong."; details are in the server log. |

Notable 409 messages: "An organisation needs at least one admin." (demoting or removing the last admin), "You cannot remove yourself.", "Keep at least one channel.", "A channel with that name already exists.", "A user with that email already exists." Emails are lowercased before the duplicate check; channel names are compared exactly (case-sensitive).

## Data shapes

**User** (`/api/me`, login): `{ "id", "email", "role", "mustChangePassword" }`. In the people list each item also has `createdAt`.

**Token**: `{ "id", "name", "prefix", "createdAt", "lastUsedAt" }` (`lastUsedAt` is null until first use). `prefix` is the first 12 characters of the secret, for recognition.

**Channel**: `{ "id", "name", "createdAt" }`.

**Key**: `{ "id", "channelId", "prefix", "createdAt", "revokedAt" }`. `revokedAt` is null while the key is active.

**Feedback**:

```json
{
  "id": "fb_01J…",
  "channelId": "ch_01J…",
  "channelName": "Default",
  "message": "Love it.\nBut the export is slow.",
  "userId": "cust-4821",
  "appVersion": "2.3.0",
  "platform": "ios",
  "metadata": { "screen": "export" },
  "status": "new",
  "receivedAt": "2026-03-01T09:30:00.000Z",
  "updatedAt": "2026-03-01T09:30:00.000Z",
  "updatedBy": null,
  "tags": ["bug", "export"]
}
```

`channelName` is null if the channel was deleted. `userId`, `appVersion`, `platform` and `metadata` are null when the submitter did not send them; `metadata` is a JSON object when present. `status` is `new`, `reviewed`, `actioned` or `ignored`. `updatedBy` is the user ID of whoever last changed the status (null if never changed). `tags` are sorted case-insensitively by name.

**Feedback detail** is Feedback plus `history`, oldest first:

```json
{ "from": "new", "to": "reviewed", "userId": "us_01J…", "userEmail": "a@example.com",
  "via": "ui", "at": "2026-03-02T10:00:00.000Z" }
```

`via` is `ui` (this API) or `mcp` (the MCP server). `userEmail` is null if that user has since been removed.

## Endpoints

"Self-host" marks routes registered by the self-host Authenticator (`internal/selfhost/auth.go`); a SaaS deployment supplies its own versions. All other routes belong to the shared API.

### Auth (self-host)

| Method and path | Role | Success |
| --- | --- | --- |
| `POST /api/auth/login` | none | 200 |
| `POST /api/auth/logout` | none needed | 204 |
| `POST /api/auth/password` | any signed-in | 204 |

`POST /api/auth/login`, body `{ "email", "password" }`. Email is trimmed and lowercased. Response `{ "user": <User> }` and the session cookie; any previous session on the browser is replaced. Wrong email or password is `401 unauthenticated` ("Email or password is incorrect."), the same message for both. Check `user.mustChangePassword` and send the user to the change-password screen if true.

`POST /api/auth/logout`: ends the session and clears the cookie. Succeeds with 204 even if there was no session.

`POST /api/auth/password`, body `{ "currentPassword", "newPassword" }`. New password is 10 to 256 characters and must differ from the current one; otherwise `400` with a message. Clears `mustChangePassword`, ends the user's other sessions and keeps the current one. Without a session: `401`.

### Me

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/me` | any signed-in | 200 `<User>` |

Also renews the session cookie. Call it on load to learn whether the user is signed in, their role, and whether they must change their password.

### Users

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/users` | admin | 200 `{ "items": [<User + createdAt>] }`, oldest first |
| `POST /api/users` (self-host) | admin | 201 |
| `PATCH /api/users/{id}` | admin | 200 `<User + createdAt>` |
| `DELETE /api/users/{id}` | admin | 204 |
| `POST /api/users/{id}/reset-password` (self-host) | admin | 200 |

`POST /api/users`, body `{ "email", "role" }` (`role` is `admin` or `reviewer`). Creates a user with a temporary password they must change at first login.

```json
{ "user": { "id": "us_01J…", "email": "new@example.com", "role": "reviewer",
            "mustChangePassword": true, "createdAt": "2026-03-01T09:30:00.000Z" },
  "temporaryPassword": "k7Rn2…" }
```

Errors: 400 "Enter a valid email address." (also over 254 characters), 400 "Role must be admin or reviewer.", 409 duplicate email. The temporary password is shown once; the admin shares it out of band.

`PATCH /api/users/{id}`, body `{ "role": "admin" | "reviewer" }`. Errors: 404 "User not found.", 409 last admin.

`DELETE /api/users/{id}`: removes the user, their sessions and their personal access tokens. Errors: 404, 409 "You cannot remove yourself.", 409 last admin.

`POST /api/users/{id}/reset-password`: no body. Returns `{ "temporaryPassword": "…" }` (once), sets `mustChangePassword`, and ends all of that user's sessions. Errors: 404, 400 "Use change password for your own account." when the id is the caller's.

### Personal access tokens

Each user manages only their own. Tokens are for the MCP server and inherit the owner's role. How to use one is in [MCP.md](MCP.md).

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/tokens` | any signed-in | 200 `{ "items": [<Token>] }`, newest first |
| `POST /api/tokens` | any signed-in | 201 |
| `DELETE /api/tokens/{id}` | any signed-in | 204 |

`POST /api/tokens`, body `{ "name" }` (1 to 64 characters after trimming; else 400). Response `{ "token": <Token>, "secret": "fbx_pat_…" }`. The secret is shown once and cannot be retrieved later. `DELETE` of an id that is not yours or does not exist is `404 not_found`.

### Channels

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/channels` | any signed-in | 200 `{ "items": [<Channel>] }`, oldest first |
| `POST /api/channels` | admin | 201 `<Channel>` |
| `PATCH /api/channels/{id}` | admin | 200 `<Channel>` |
| `DELETE /api/channels/{id}` | admin | 204 |

`POST` and `PATCH` take `{ "name" }`: 1 to 64 characters after trimming (400 otherwise), unique per org, compared exactly (409). `PATCH` on an unknown id is 404.

`DELETE` revokes all the channel's keys immediately, deletes the channel, and keeps its feedback, which then shows `channelName: null`. 404 if unknown; 409 "Keep at least one channel." if it is the last one.

### Ingest keys (admin)

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/keys[?channel={id}]` | admin | 200 `{ "items": [<Key>] }`, newest first, revoked keys included |
| `POST /api/channels/{id}/keys` | admin | 201 |
| `POST /api/keys/{id}/revoke` | admin | 200 `<Key>` |

`POST /api/channels/{id}/keys` has no body. Response `{ "key": <Key>, "secret": "fbx_…" }`. The secret is shown once; only its hash is stored. 404 for an unknown channel. `GET` with an unknown `channel` is 404.

`POST /api/keys/{id}/revoke` has no body and takes effect for ingest immediately. Revoking an already revoked key succeeds and returns the key. The key created at first boot can be revoked like any other. 404 for an unknown key.

### Feedback

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/feedback` | any signed-in | 200 `{ "items": [<Feedback>], "nextCursor": string or null }` |
| `GET /api/feedback/{id}` | any signed-in | 200 `<Feedback detail>` |
| `PATCH /api/feedback/{id}` | any signed-in | 200 `<Feedback>` |
| `DELETE /api/feedback/{id}` | admin | 204 |
| `POST /api/feedback/status` | any signed-in | 200 `<Bulk result>` |
| `POST /api/feedback/add-tags` | any signed-in | 200 `<Bulk result>` |
| `POST /api/feedback/remove-tags` | any signed-in | 200 `<Bulk result>` |
| `POST /api/feedback/delete-by-user` | admin | 200 `{ "deleted": n }` |
| `GET /api/feedback/export` | any signed-in | 200 file stream |

Single-item routes answer `404 not_found` ("Feedback not found.") for an unknown id.

`PATCH /api/feedback/{id}`, body `{ "status": "new" | "reviewed" | "actioned" | "ignored" }`. Returns the updated feedback (without `history`). Setting the status it already has is a no-op and writes no history. 400 "Unknown status." otherwise.

`DELETE /api/feedback/{id}`: permanent. Deleted feedback still counts toward the month's plan limit.

`POST /api/feedback/delete-by-user`, body `{ "userId" }` (exact match; 400 if empty). Permanent. Response `{ "deleted": 12 }`; 0 is not an error.

#### List filters

`GET /api/feedback`, `GET /api/feedback/export` and (for `from`/`to` only) `GET /api/stats` use these query parameters. All are optional and combine with AND.

| Parameter | Meaning |
| --- | --- |
| `status` | `new`, `reviewed`, `actioned` or `ignored`. Repeat the parameter and/or comma-separate: `status=new&status=reviewed` or `status=new,reviewed`. Several statuses combine with OR. Unknown value: 400 "Unknown status." |
| `channel` | Channel ID. |
| `tag` | Tag name, case-insensitive. |
| `userId` | Exact match on the submitter's opaque customer ID. |
| `from` | Received at or after this time (inclusive). |
| `to` | Received before this time (exclusive). `from` after `to` is 400. |
| `q` | Case-insensitive substring search in the message; at most 200 characters (400 otherwise). |

List only: `limit` (1 to 200, default 50; 400 otherwise) and `cursor`.

#### Pagination

Order is received time descending, then ID descending. `nextCursor` is an opaque string, `null` on the last page. Pass it back unchanged as `cursor` together with the same filters. A malformed cursor is 400 "Invalid cursor." Do not build or inspect cursors.

```json
{ "items": [ /* up to limit Feedback objects */ ], "nextCursor": "eyJyIjoxNzQwMD…" }
```

#### Bulk operations

All three bulk routes take `ids`: 1 to 500 IDs. Empty strings and duplicates are ignored; no IDs left is 400 "Choose at least one feedback item.", more than 500 is 400. Response:

```json
{ "updated": 2, "notFound": ["fb_missing"] }
```

`updated` is the number of given IDs that exist (an item already in the requested state counts, but gets no history entry); `notFound` lists the rest. Unknown IDs are not an error.

- `POST /api/feedback/status`: `{ "ids": [...], "status": "reviewed" }`. Each change is recorded in the item's history with the user and `via: "ui"`.
- `POST /api/feedback/add-tags`: `{ "ids": [...], "tags": ["bug", "export"] }`. Creates tags that do not exist; an existing tag matches case-insensitively and keeps its original spelling.
- `POST /api/feedback/remove-tags`: same body. Removes the tags from those items; unknown tag names are ignored.

`tags` holds 1 to 20 names of 1 to 64 characters each after trimming (400 otherwise).

#### Export

`GET /api/feedback/export?format=csv|json` plus the filters above (no `limit` or `cursor`; all matching items, newest first). `format` is required: 400 "Format must be csv or json." Navigate to it or fetch it as a download; it is a GET and needs no CSRF header.

- Headers: `Content-Disposition: attachment; filename="feedback-YYYYMMDD.csv"` (or `.json`, date is UTC today), `Content-Type: text/csv; charset=utf-8` or `application/json`.
- JSON: one array of Feedback objects, same shape as the list (no `history`). No matches gives `[]`.
- CSV: RFC 4180 quoting, CRLF line endings, header row then one row per item. Columns: `id`, `received_at`, `status`, `channel`, `user_id`, `platform`, `app_version`, `tags`, `message`, `metadata`. `channel` is the channel name (empty if deleted), `tags` are joined with `; `, `metadata` is the JSON text, nulls are empty cells. Cells in the text columns that start with `=`, `+`, `-`, `@`, tab or carriage return get a leading `'` so spreadsheets do not run them as formulas (`received_at`, `status` and `metadata` are not altered).
- Validation errors (format, filters) come back as a normal JSON error before the download starts. If a failure happens after streaming has begun, the server aborts the connection instead of finishing the file, so a broken download looks like a network error, not a complete but truncated file. Show a generic "export failed" message.

### Tags

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/tags` | any signed-in | 200 `{ "items": [{ "id", "name", "count" }] }`, sorted by name case-insensitively |

`count` is the number of feedback items with the tag.

### Stats

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/stats[?from=&to=]` | any signed-in | 200 |

Only `from` and `to` are read (same meaning as above); other filters are ignored.

```json
{
  "total": 130,
  "byStatus": { "new": 90, "reviewed": 30, "actioned": 10 },
  "byChannel": [ { "channelId": "ch_01J…", "name": "Default", "count": 130 } ],
  "byTag": [ { "name": "bug", "count": 12 } ]
}
```

`byStatus` omits statuses with no items. `byChannel` and `byTag` are sorted by count descending; `name` in `byChannel` is null for a deleted channel.

### Ops

| Method and path | Role | Success |
| --- | --- | --- |
| `GET /api/ops` | admin | 200 |

```json
{
  "queue": { "depth": 0, "oldestAgeSeconds": 0, "dlq": 0 },
  "counters": { "day": 14, "month": 212, "dayKey": "…", "monthKey": "…" },
  "dbSizeBytes": 1048576,
  "version": "0.2.0",
  "mode": "all",
  "dialect": "sqlite"
}
```

`queue.depth` is the number of accepted submissions not yet stored; `oldestAgeSeconds` is the age of the oldest (0 when empty); `dlq` is the dead-letter count. `counters.day` and `counters.month` are this org's accepted submissions in the current day and billing month (they are flushed on `COUNTER_FLUSH_INTERVAL`, so they can lag a few seconds); the keys identify the periods. `dialect` is `sqlite` or `postgres`. A failure reading the queue is `500 internal`.

## Semantics for the UI

- **Secrets are shown once**: ingest key (`secret`), personal access token (`secret`) and temporary passwords (`temporaryPassword`). Only hashes or prefixes are stored; show the value with a copy button and warn that it cannot be shown again.
- **Deleting a channel** revokes its keys and keeps its feedback (`channelName: null`). Warn the admin that apps using those keys will start failing.
- **Deleted feedback still counts** toward the month's submission limit.
- **Feedback text is plain text.** Preserve line breaks (for example `white-space: pre-wrap`) and never render it, `metadata` values or tag names as HTML or Markdown.
- **`userId` is an opaque customer ID**, not an email. Show it as given.
- **Statuses** always display as icon plus label; the API only sends the four lowercase names.
