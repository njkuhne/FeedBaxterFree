# MCP server

FeedBaxter has an MCP server so your own AI client can read and triage the feedback in your workspace. FeedBaxter runs no AI itself: your client decides what to do, and the server only lists, reads, tags and sets statuses, as the user whose token you gave it.

- URL: `https://<your host>/mcp`
- Transport: Streamable HTTP, stateless, with plain JSON responses. There are no sessions and no streaming; every request stands alone. `GET` and `DELETE` on `/mcp` answer `405`.
- Authentication: a personal access token in `Authorization: Bearer fbx_pat_…` on every request.

## Create a token

Sign in, open your profile and create a token under Personal access tokens. The API does the same with `POST /api/tokens` (see [API.md](API.md)). The token is shown once and cannot be retrieved later, so copy it when it appears.

A token acts as you and has your role. Revoke it at any time from the same place; the next request that uses it fails. Removing a user revokes their tokens. Tokens work on `/mcp` only, never on `/api`.

## Connect a client

Claude Code:

```sh
claude mcp add --transport http feedbaxter https://feedback.example.com/mcp --header "Authorization: Bearer fbx_pat_YOUR_TOKEN"
```

Claude Desktop and other clients that read a JSON config:

```json
{
  "mcpServers": {
    "feedbaxter": {
      "type": "http",
      "url": "https://feedback.example.com/mcp",
      "headers": { "Authorization": "Bearer fbx_pat_YOUR_TOKEN" }
    }
  }
}
```

The client must be able to send a custom `Authorization` header. Clients that only support OAuth sign-in cannot connect (see [BACKLOG.md](BACKLOG.md)).

## Tools

Dates are RFC 3339, for example `2026-03-01T00:00:00Z`. Times in results look like `2026-03-01T12:00:00.000Z`. Optional parameters may be left out.

| Tool | What it does | Parameters | Result |
| --- | --- | --- | --- |
| `list_feedback` | List feedback, newest first, filtered by status, channel, tag, user ID, date range or search text. | All optional: `status` (list of `new`, `reviewed`, `actioned`, `ignored`), `channel` (channel ID), `tag` (tag name), `userId`, `from` (received at or after), `to` (received before), `q` (text in the message, at most 200 characters), `limit` (1 to 200, default 50), `cursor` (the `nextCursor` of the previous page) | `{ items: [feedback], nextCursor }`. `nextCursor` is null on the last page. |
| `get_feedback` | Get one feedback item with its metadata, tags and status history. | `id` (required) | The feedback item plus `history`: a list of `{ from, to, userId, userEmail, via, at }`. |
| `list_tags` | List the tags in use and how many feedback items have each. | None | `{ items: [{ id, name, count }] }` |
| `set_status` | Set the status of up to 500 feedback items to new, reviewed, actioned or ignored. | `ids` (required, at most 500), `status` (required) | `{ updated, notFound }` |
| `add_tags` | Add tags to up to 500 feedback items, creating tags that do not exist yet. | `ids` (required, at most 500), `tags` (required, 1 to 20 names of 1 to 64 characters) | `{ updated, notFound }` |
| `remove_tags` | Remove tags from up to 500 feedback items. | `ids` (required, at most 500), `tags` (required, 1 to 20 names) | `{ updated, notFound }` |
| `stats` | Count feedback by status, channel and tag for a date range. | `from`, `to` (both optional; `from` must not be after `to`) | `{ total, byStatus, byChannel: [{ channelId, name, count }], byTag: [{ name, count }] }` |

A feedback item has the same fields as in the API: `id`, `channelId`, `channelName`, `message`, `userId`, `appVersion`, `platform`, `metadata` (an object or null), `status`, `receivedAt`, `updatedAt`, `updatedBy` and `tags`. Fields that have no value are null; `tags` is an empty list, never null.

In bulk results, `updated` counts the items found, including ones already in the requested state. `notFound` lists the IDs that do not exist; they are skipped and the rest are applied. Repeated IDs count once. Removing a tag leaves the tag itself in the tag list with a count of zero.

## Roles

All seven tools work for both admins and reviewers. Deleting feedback, managing channels, keys and people, and everything else only admins can do are not available over MCP.

## Attribution

A status change made over MCP shows in the feedback's history with `via: mcp` and the user who owns the token (their ID and email), next to changes made in the UI.

## Errors

- A missing, malformed, unknown or revoked token is an HTTP `401` with `WWW-Authenticate: Bearer` and a JSON body `{ "error": "unauthenticated", "message": "…" }`. Auth is checked on every request, so a revoked token stops working at once, even in the middle of a conversation.
- Any other path under `/mcp/` is `404 not_found`.
- Bad input, unknown IDs, more than 500 IDs, a bad cursor, a bad date or an unknown status come back as a tool error: the call succeeds at the protocol level, `isError` is true and the text is a plain sentence such as "Invalid cursor." or "Feedback not found."
- Anything else is a tool error that says "Something went wrong." The details are in the server log, not in the reply.

## Plain text

Feedback text is returned verbatim, with line breaks preserved. It is never rendered or interpreted by FeedBaxter. It comes from your customers, so a client that passes it to an AI should treat it as untrusted text, not as instructions.
