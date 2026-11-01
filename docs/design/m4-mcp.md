# M4 brief: `internal/mcp`, the MCP server

Author: Opus director · Milestone M4 · Source: ARCHITECTURE §1, §2, §9, §10, §12; ADR 0006; SPEC "MCP server"; m2-service.md, m2-api.md

Streamable HTTP MCP server at `/mcp` in api and all modes. Thin wrappers over `internal/service`, which
owns every role check and the `via` attribution. Files: `internal/mcp/{mcp.go,auth.go,tools.go,types.go}`
+ `internal/mcp/mcp_test.go` (+ `helpers_test.go` if it helps); `feedbaxter.go` (mount);
`feedbaxter_test.go` (one wiring test, see Tests: the only file outside the M4 set, because only Run proves
the mount); `go.mod`/`go.sum`; docs (part 2). Nothing in `ext/`, `internal/service`, `internal/api` or
`internal/ui` changes. No new env vars.

## Dependency — decision record

`github.com/modelcontextprotocol/go-sdk v1.8.0`, package `.../mcp` (import it as `sdk` to avoid the clash
with our package name). Its transitive modules (`google/jsonschema-go`, `segmentio/encoding`,
`segmentio/asm`, `yosida95/uritemplate/v3`, `golang.org/x/oauth2`, …) come in via `go mod tidy`; nothing
else is added. Measured with a probe that links a server, one typed tool and the streamable handler:
linux/amd64 27,144,352 → 28,836,000 bytes (+1.6 MiB, limit 33,554,432); linux/arm64 27,000,992.

## Exported surface

```go
package mcp
// New returns the handler for exactly the path /mcp. It builds one sdk.Server with the seven tools and
// one sdk.StreamableHTTPHandler, both shared by every request. log nil = slog.Default().
func New(svc *service.Service, log *slog.Logger) http.Handler
```

`feedbaxter.go`, inside the existing `if cfg.Mode == "api" || cfg.Mode == "all"` block, next to
`mux.Handle("/api/", apiHandler)`: build `svc` once (hoist it out of the `if writesKeys` block so the API
and MCP share one `*service.Service`), build `mcpHandler := mcp.New(svc, log)` once and mount it at both
`/mcp` and `/mcp/`. The handler's own path check then answers every sub-path, for any method, with a JSON 404;
without the second pattern, sub-paths fall through to the UI handler, which answers POST with 405. It is a sibling of `/api/`, never behind `internal/api`'s CSRF or cookie code.

## Request pipeline (`mcp.go`, `auth.go`)

`New` returns a `handler` whose ServeHTTP does, in order:

1. `w.Header().Set("Cache-Control", "no-store")`.
2. `r.URL.Path != "/mcp"` → 404 `{"error":"not_found","message":"Not found."}` (defence in depth if someone
   mounts it at `/mcp/`).
3. Auth: header `Authorization`, scheme `Bearer` matched case-insensitively, then one space, then the token
   (`strings.TrimSpace`). Missing header, other scheme or empty token → 401 `unauthenticated` "Send a
   personal access token in the Authorization header." with `WWW-Authenticate: Bearer`. Otherwise
   `svc.PrincipalFromToken(r.Context(), tok)`: `service.KindOf(err) == KindUnauthenticated` → 401
   `unauthenticated` with the service message ("Token is invalid or revoked.") and
   `WWW-Authenticate: Bearer error="invalid_token"`; any other error → log at error
   (`"mcp: resolve token"`, never the token) and 500 `{"error":"internal","message":"Something went wrong."}`.
4. `r = r.WithContext(ext.WithPrincipal(r.Context(), p))`, then the SDK handler.

Error bodies: same shape as the API, `{"error":code,"message":sentence}`, `Content-Type:
application/json`, written by a small local `writeError(w, status, code, msg)` (do not import
`internal/api`; it would drag the cookie/CSRF package into MCP for three lines).

No session cookie path, ever: the handler never looks at cookies. The token's principal has `Via: "mcp"`
already (service), so history attribution needs nothing here. There is no rate limit on `/mcp` (same as
`/api`); see BACKLOG.

SDK construction (exact names, verified against v1.8.0 source):

```go
srv := sdk.NewServer(&sdk.Implementation{Name: "feedbaxter", Title: "FeedBaxter", Version: svc.Version},
    &sdk.ServerOptions{Instructions: "Read and triage the app feedback in this FeedBaxter workspace."})
// sdk.AddTool(srv, &sdk.Tool{...}, handler) × 7
h := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{
    Stateless:                    true,  // no Mcp-Session-Id, no session map; GET/DELETE → 405 Allow: POST
    JSONResponse:                 true,  // application/json, not text/event-stream
    Logger:                       log,
    MaxRequestBodyBytes:          1 << 20, // same 1 MiB as /api; 500 IDs is ~15 KB
    PropagateRequestCancellation: true,  // cancel tool ctx when the client goes away (2026-07-28+ clients)
    DisableLocalhostProtection:   true,  // see below
})
```

**Why `DisableLocalhostProtection`** (the one non-default): the SDK's DNS-rebinding guard returns 403
when the connection's local address is loopback and `Host` is not. That breaks a self-hoster who runs the
binary (or `network_mode: host`) behind a reverse proxy on the same machine: the proxy connects to
127.0.0.1 with `Host: feedback.example.com`. The guard protects unauthenticated local servers; ours
requires a bearer token on every request, which a rebinding page cannot have, so the guard adds nothing.
No `CrossOriginProtection` for the same reason (no cookies). No CORS headers: browser-based MCP clients are
out of scope for v1 (BACKLOG).

Stateless means each POST builds a throwaway `ServerSession` on `r.Context()` (which carries the principal;
the SDK detaches cancellation but keeps values) and closes it when the response is written. Nothing grows
per request: no maps in our code, and the SDK's session map is not used in stateless mode.

## Tools (`tools.go`, `types.go`)

Every handler has the shape `func(ctx context.Context, _ *sdk.CallToolRequest, in X) (*sdk.CallToolResult, Y, error)`,
gets the principal with `p, _ := ext.PrincipalFrom(ctx)` (zero principal if absent; the service then
answers unauthenticated, which maps to a tool error), calls exactly one service method (two for
nothing), converts, and returns `nil, out, nil` or `nil, Y{}, toolErr(...)`. **No role checks, no
`Via` checks, no limit checks that the service already does** (ID count, tag count, limit range, statuses,
cursor, q length, date order are all the service's). The only validation here is parsing RFC 3339 strings.

| Tool | Description (exact) | Input → output | Annotations |
| --- | --- | --- | --- |
| `list_feedback` | List feedback, newest first, filtered by status, channel, tag, user ID, date range or search text. | `{status?:[string], channel?, tag?, userId?, from?, to?, q?, limit?:int, cursor?}` → `{items:[feedback], nextCursor:string\|null}` | ReadOnly |
| `get_feedback` | Get one feedback item with its metadata, tags and status history. | `{id}` → feedback + `history:[{from,to,userId,userEmail,via,at}]` | ReadOnly |
| `list_tags` | List the tags in use and how many feedback items have each. | `{}` → `{items:[{id,name,count}]}` | ReadOnly |
| `set_status` | Set the status of up to 500 feedback items to new, reviewed, actioned or ignored. | `{ids:[string], status}` → `{updated, notFound:[string]}` | Idempotent, DestructiveHint false |
| `add_tags` | Add tags to up to 500 feedback items, creating tags that do not exist yet. | `{ids, tags:[string]}` → bulk | Idempotent, DestructiveHint false |
| `remove_tags` | Remove tags from up to 500 feedback items. | `{ids, tags}` → bulk | Idempotent, DestructiveHint false |
| `stats` | Count feedback by status, channel and tag for a date range. | `{from?, to?}` → `{total, byStatus:{status:n}, byChannel:[{channelId,name,count}], byTag:[{name,count}]}` | ReadOnly |

Every tool sets `Annotations: &sdk.ToolAnnotations{...}` and `OpenWorldHint: ptr(false)`.

Input structs: every field has a `json` tag (camelCase as above) and a `jsonschema:"<plain sentence>"`
description (must not start with `WORD=`). Optional fields carry `,omitempty` (that is what makes them
optional in the inferred schema); required: `get_feedback.id`, `set_status.ids`/`status`,
`add_tags`/`remove_tags` `ids`/`tags`. Descriptions say the limits and formats, e.g. `limit`: "Page size,
1 to 200. Defaults to 50."; `from`: "Only feedback received at or after this time, as an RFC 3339 date such
as 2026-03-01T00:00:00Z."; `cursor`: "The nextCursor from the previous page."; `status` (list): "Statuses to
include: new, reviewed, actioned or ignored."; `ids`: "Feedback IDs, at most 500.". The SDK validates
input against the schema before the handler (unknown properties and wrong types become a tool error from
the SDK); that is fine.

`list_feedback` semantics mirror `GET /api/feedback`: `service.Filter{Statuses: in.Status, ChannelID:
in.Channel, Tag, UserID, Q}`, `From`/`To` parsed; `limit` 0/absent = service default; pass it straight
to `ListFeedback` (the service rejects <0 and >200). The API's extra "explicit 0 is an error" rule does
not apply (JSON omitempty cannot tell 0 from absent; the description says "1 to 200").

Dates: `time.Parse(time.RFC3339, s)`; failure → tool error "Use an RFC 3339 date for from, for example
2026-03-01T00:00:00Z." (name = `from`/`to`), the same sentence as the API.

Output structs (`types.go`) are our own, not `internal/api`'s (those are unexported, and the SDK derives
an output schema from them, which `json.RawMessage` breaks): same field names and formats as the API
JSON — times `2006-01-02T15:04:05.000Z` UTC, optional strings as `*string` (null), lists never null
(`[]`), `byStatus` never null. **`metadata` is `any`**: unmarshal `store.Feedback.Metadata` (raw JSON
object or empty) into `any`; empty → `nil` (JSON null). A `json.RawMessage` field would infer an
"array of integers" schema and every call with metadata would fail output validation. Output fields need
`json` tags; `jsonschema` descriptions on outputs are optional (add them where they help a model:
`nextCursor`, `notFound`, `via`).

Bulk tools return `service.BulkResult` as `{updated:int, notFound:[string]}` (`notFound` never null).

## Error mapping

```go
// toolErr turns a service error into the error a typed tool handler returns, which the SDK packs into a
// CallToolResult with IsError true and the message as text. Never leaks internals.
func (h *handler) toolErr(tool string, err error) error {
    var se *service.Error
    if errors.As(err, &se) && se.Kind != 0 { return errors.New(se.Msg) }  // every Kind: message is caller-safe
    h.log.Error("mcp: tool failed", "tool", tool, "err", err)
    return errors.New("Something went wrong.")
}
```

Every error a handler returns goes through `toolErr` (date-parse errors are constructed as
`*service.Error{Kind: KindInvalid}` so they take the same path). Never return a `*jsonrpc.Error`. Never put
input values (IDs, query text) in the log line.

## Tests (`internal/mcp/mcp_test.go`)

Harness: `storetest.ForEachDialect`; tenant, channel, admin and reviewer via `store.Create*` like
`internal/api/helpers_test.go`; `svc := &service.Service{DB: db, Log: log, Version: "test", Mode: "all"}`
(no Queue/Keys needed: no MCP tool touches them); PATs via `svc.CreateToken(ctx, principal, "test")`;
feedback seeded with `store.InsertFeedbackBatch` (ids `fb_0001…`, one row with `Metadata`
`{"plan":"pro"}`, rows a day apart). Server: `httptest.NewServer(mux)` with `mux.Handle("/mcp",
mcp.New(svc, log))` and `mux.Handle("/mcp/", sameHandler)` (so the handler's own 404 is exercised).
Client: `sdk.NewClient(&sdk.Implementation{Name: "test"}, nil).Connect(ctx,
&sdk.StreamableClientTransport{Endpoint: srv.URL + "/mcp", HTTPClient: &http.Client{Transport:
bearer(tok)}, DisableStandaloneSSE: true}, nil)`, where `bearer` is a RoundTripper that sets the header on
a clone of the request. Close sessions in `t.Cleanup`. Raw HTTP (not the SDK client) for the 401/404
tests. Decode tool output from `res.StructuredContent` (marshal + unmarshal into the output struct or a
map).

| Test | Proves |
| --- | --- |
| `TestUnauthenticated` (table) | POST /mcp with a valid initialize body and: no header; `Basic …`; `Bearer ` empty; malformed `Bearer abc`; well-formed but unknown `fbx_pat_` + 32×`a` → 401, body exactly `{error:"unauthenticated", message:<non-empty>}`, `WWW-Authenticate` starts with `Bearer`, `Cache-Control: no-store`. Also GET /mcp without a token → 401 (auth precedes the SDK). |
| `TestSubPathNotFound` | `/mcp/x` and `/mcp/` with a valid token → 404 `not_found` JSON; the SDK never sees it. |
| `TestStatelessJSON` | Raw POST initialize with a valid token, `Accept: application/json, text/event-stream` → 200, `Content-Type: application/json`, no `Mcp-Session-Id` header. GET with a valid token → 405. Proves Stateless + JSONResponse. |
| `TestListTools` | `tools/list` returns exactly the seven names; each has a non-empty description, an input schema of type object and an output schema; `set_status` input schema has `ids` and `status` in `required`; `list_feedback` has none required. |
| `TestListFeedback` | Round trip: all items newest first with API field shapes (receivedAt format, tags `[]`, metadata object on the one row and null on others); limit 2 + nextCursor pages through 5 rows with no overlap and a final null cursor; status, tag (after add_tags), q, from/to filters; `limit: 201` → tool error; `from: "yesterday"` → tool error with the RFC 3339 sentence. |
| `TestBadCursor` | `cursor: "!!!"` → `IsError` true, text "Invalid cursor.", and the call itself returns no Go error (not a protocol error, not a 500). |
| `TestGetFeedback` | Full detail incl. metadata, tags and history after a set_status; unknown id → tool error "Feedback not found.". |
| `TestSetStatusViaMCP` | set_status on 2 ids + 1 unknown → `{updated:2, notFound:[unknown]}`; get_feedback history entry has `via:"mcp"`, the caller's userId and email; the row's `updatedBy` is the caller. |
| `TestTags` | add_tags creates a new tag; list_tags shows it with the right count; remove_tags drops it from items (count 0 or gone, whatever the service does); bad tag (65 chars) → tool error. |
| `TestStats` | totals by status, channel (with name) and tag over a range; `from` after `to` → tool error. |
| `TestBulkLimit` | 501 distinct ids to set_status, add_tags and remove_tags → tool error mentioning 500; 500 ids → success with `notFound` listing the unknown ones. |
| `TestReviewerCanUseEveryTool` | With a reviewer's PAT, all seven tools succeed (none is `IsError`). |
| `TestRevokedToken` | Connect and call list_tags with a token; `svc.RevokeToken`; the next call fails (Go error from the client, since the HTTP answer is 401); a raw POST with that token → 401. Proves auth is per request, not per session. |
| `TestInternalErrorNotLeaked` | SQLite only. A second `service.Service` whose `DB` is a `store.Open`ed temp SQLite file that has then been closed (never close the shared test DB: storetest cleanup needs it); POST /mcp with a well-formed token → 500 `internal` "Something went wrong.", no driver text in the body, log has `mcp: resolve token` and not the token. |
| `TestToolErrMapping` (internal, `package mcp`, `tools_internal_test.go`) | `toolErr`: each service Kind → its message verbatim; a wrapped `*service.Error` too; a plain error → "Something went wrong." and one error log line naming the tool. |
| `TestNoGoroutineGrowth` | After warm-up, 50 tool calls on one client session; close idle connections; `runtime.NumGoroutine()` returns to within +5 of the baseline within 2 s (poll). Guards against per-request session leaks. |

`feedbaxter_test.go`: `TestRunAllMountsMCP` — `startRun` in all mode, POST `/mcp` with no token → 401
`unauthenticated`; POST `/mcp/x` → 404 JSON. (No token flow here; internal/mcp covers it.)

## Acceptance (part 1)

`make vet`, `make test`, `make test-pg` green; `go mod tidy` leaves go.mod/go.sum unchanged;
`make size-check` under the limit; `make image` builds. `git status` shows only the files named above.

## Part 2: docs

- `docs/MCP.md` (new): what it is (one paragraph: your own AI client reads and triages feedback;
  FeedBaxter runs no AI); URL `https://<your host>/mcp`, Streamable HTTP, stateless, JSON responses;
  creating a token (Settings → Personal access tokens in the UI, or `POST /api/tokens`), shown once,
  inherits the owner's role, revoke any time; header `Authorization: Bearer fbx_pat_…`; examples with a
  placeholder token: Claude Code `claude mcp add --transport http feedbaxter https://feedback.example.com/mcp --header "Authorization: Bearer fbx_pat_YOUR_TOKEN"`
  and a Claude Desktop / generic JSON config (`{"mcpServers":{"feedbaxter":{"type":"http","url":…,"headers":{"Authorization":"Bearer …"}}}}`);
  tool table (name, what it does, parameters with limits, result shape); roles (all seven work for
  admins and reviewers; deleting feedback and admin work are not available over MCP); attribution
  (status changes show `via: mcp` and the user); errors (HTTP 401 with `WWW-Authenticate: Bearer` for a
  missing, invalid or revoked token; tool errors with a plain message for bad input, unknown IDs, >500 IDs,
  bad cursor; "Something went wrong." otherwise); plain text: feedback text is returned verbatim, never
  rendered. Plain voice, sentence case, no marketing.
- `README.md`: link `docs/MCP.md` in the Documentation list.
- `docs/API.md`: the token sentence and the Personal access tokens section point at `docs/MCP.md`.
- `docs/ARCHITECTURE.md` §10: add one sentence recording stateless JSON-response Streamable HTTP and the
  disabled localhost guard (with the reason in a clause); otherwise unchanged.
- `docs/BACKLOG.md`: rows for (a) no rate limit on `/mcp` or `/api` per token, (b) no CORS on `/mcp`, so
  browser-based MCP clients cannot connect, (c) no OAuth (PAT only; clients that insist on OAuth discovery
  cannot connect), (d) MCP has no export tool (list_feedback pages instead).
