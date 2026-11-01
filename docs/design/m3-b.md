# M3 brief B: inbox, detail, per-user, onboarding, empty states, keyboard, bulk, export

Author: Opus director · Milestone M3 · Design source: `docs/design/m3-ui-design.md` §3 (inbox row, filter bar, bulk bar, load more, empty state, code block, secret once, email warning) and §4 (Inbox, Onboarding, Feedback detail, Per-user view). Copy there is binding word for word.

Brief A is done: build on `web/src/components/*`, `web/src/lib/*` (api, endpoints, types, filters, cursorList, shortcuts, time, email, format, clipboard), `web/src/shell/*`, `usePageTitle`. Reuse; do not re-implement a shared component. If a shared component needs a new prop, add it compatibly and say so in the report. Brief C runs in parallel and owns `pages/Channels.tsx`, `pages/People.tsx`, `pages/Settings.tsx`; do not touch those. Brief D runs in parallel on Dockerfile/CI/docs; do not touch those.

## Files

```
web/src/pages/Inbox.tsx            # gate: stats.total === 0 → <Onboarding/>, else the inbox
web/src/pages/Onboarding.tsx
web/src/pages/FeedbackDetail.tsx
web/src/pages/PerUser.tsx
web/src/feedback/FeedbackList.tsx  # the row list + roving focus + keyboard + selection, shared by Inbox and PerUser
web/src/feedback/FeedbackRow.tsx
web/src/feedback/FilterBar.tsx
web/src/feedback/BulkBar.tsx
web/src/feedback/ShortcutHelp.tsx
web/src/feedback/ExportLinks.tsx
web/src/feedback/useFeedbackList.ts # cursorReducer + fetching + in-memory cache keyed by query (back restores list and focus)
web/src/feedback/statusCopy.ts      # labels and announce sentences ("Marked 3 as reviewed.")
web/src/feedback/*.test.tsx / *.test.ts
web/src/styles/feedback.css         # imported once from main.tsx or the pages
```
Small changes allowed: `lib/shortcuts.ts` (see below), `shell/Rail.tsx` (Inbox current on `/`, `/feedback/*`, `/user/*`: set `aria-current="page"` on those paths, not only exact `/`).

## Behaviour (summary; the design doc is the full spec)

- **Inbox** reads filters from the URL (`filtersFromQuery`), writes them back with `setSearchParams` (replace for typing in search, push for status/tag/channel). Loads `stats`, `channels`, `tags` on mount; counts on the segmented control come from `stats.byStatus` (All = `stats.total`). Channel select only with ≥ 2 channels; channel name in rows only then.
- **Rows** exactly as design §4 Inbox (grid, preview first 200 chars with newlines collapsed, metadata line with status word first, up to 3 pills + "+N", `<time dateTime title>` with `relativeTime`/`absoluteTime`). Selected row: class with dusk tint background and metadata in ink soft. No oat hover.
- **Roving focus** and keyboard as specified. `shortcuts.ts` change: inputs of `type` checkbox or radio do not count as typing (add `type?: string` to `KeyLike.target`), so j/k/x work while a row checkbox has focus; Space on a checkbox stays native. In the search field Escape blurs to the current row. Shortcut handling is a `keydown` listener on the list container plus a document listener for `/` and `?` (ignore when typing).
- **Status changes**: single item → `setStatus`; selection → `bulkStatus`. Remove rows that no longer match the status filter (status filter `all` keeps them and replaces the item's status). Move focus as specified. Announce with `useAnnounce` ("Marked 3 as reviewed.", "Marked 1 as reviewed.", plus " 1 was no longer there." when `notFound` is non-empty). Refresh stats after. Errors → `toast.show(errorMessage(e), "error")`.
- **Bulk bar** (`role="toolbar"`, aria-label "Bulk actions") only when ≥ 1 selected; actions disabled with "Updating N…" while a request runs. Bulk add tag via `addTags`; announce "Added tag bug to 3."; refresh tags. Validation 1–64 characters → inline error "Tags are 1 to 64 characters.".
- **Select all on this page** header checkbox with indeterminate state (set `el.indeterminate` via ref).
- **Load more** text action + "Showing N" in ink muted; `j` at the last row loads more when a cursor exists.
- **Export**: `<a download>` links built with `exportUrl(filters, format)` from endpoints (add it there if brief A did not: `/api/feedback/export?format=…&<toApiParams>`). Per-user view passes `userId`.
- **Back from detail**: `useFeedbackList` keeps the last list per query key in a module-level `Map` (items, nextCursor, focused id); on return show it immediately, then refresh the first page in the background without losing the focused row. "← Inbox" on detail uses `navigate(-1)` when `location.key !== "default"` and the previous route was the inbox (store a flag in `location.state` when opening from a list), else `navigate("/")`.
- **Detail**: everything in design §4 Feedback detail. Message element: `<p className="message">` with `white-space: pre-line; overflow-wrap: anywhere; font-size: var(--fs-message); line-height: var(--lh-message); max-width: var(--measure); color: var(--ink)`, content as a text child. Metadata `<dl>`: string values as-is, other values `JSON.stringify(value)`; key and value as text. History: first entry "Received · <absolute>", then one `<li>` per change. Primary button rule (new → "Mark reviewed", reviewed → "Mark actioned", else none). Keyboard e/a/i/n on the page (same `shortcutFor`, ignoring list actions). Admin delete with ConfirmDialog. 404 state.
- **Per-user**: design §4. Title "Feedback from <userId>". Uses `FeedbackList` with filters `{status:"all", userId}`, no filter bar, same keyboard. Delete all (admin) with ConfirmDialog; toast "Deleted 12 feedback items." / "Deleted 1 feedback item.". Empty: "No feedback from this user."
- **Onboarding**: design §4 Onboarding, including the Web SDK placeholder section and 5 s polling of `stats` while visible (`document.visibilityState`), switching to the inbox and toasting "First feedback received.". Curl command built from `window.location.origin`, using the created secret when there is one, else `fbx_your_key`. Reviewer sees "Ask an admin for the ingest key." and no button. The primary "Create a key" is the only primary on the page; after creation it is hidden and SecretOnce shows.
- **Empty states**: inbox zero ("No new feedback" / "New feedback appears here as it arrives.") only when status is new and tag, channel and q are empty; otherwise "Nothing matches these filters" with "Clear filters". After a status change empties the list, focus moves to the empty-state heading (`headingRef`).
- **Monthly limit reached**: the API exposes no limit state to the UI (`/api/ops` has counters but not limits), so do not build it. The director records this as an open issue.

## Styling rules

Only tokens. Feedback preview `--fs-preview` ink; metadata `--fs-meta` ink muted; time right-aligned `--fs-meta` ink muted, `white-space: nowrap`. Row padding 12 px 0, hairline bottom border, min height 64 px. Current row: 3 px dusk deep inset bar on the left of the row (`box-shadow: inset 3px 0 0 var(--dusk-deep)`) when the row link has focus (`:focus-within`). Filter bar wraps; at 768 px everything fits without horizontal scroll. Detail sections: `<h2 className="section-title">` "Tags", "Metadata", "History".

## Tests (Vitest + @testing-library/react with fireEvent, `fetch` mocked)

- `FeedbackList.test.tsx`: renders rows as `<li>` with link names = preview; j/k move focus between row links; x toggles selection and the bulk bar appears with "1 selected"; Escape clears; e on the current row calls PATCH with `reviewed` and removes the row under the new filter, announcing "Marked 1 as reviewed."; with two selected, a calls the bulk endpoint with both ids; shortcuts ignored while the search input has focus; j works while a row checkbox has focus.
- `FeedbackRow.test.tsx`: message with `<b>hi</b>` renders as literal text (no element); status word present; channel shown only when `showChannel`.
- `FeedbackDetail.test.tsx`: message preserves line breaks (computed class `message`, text includes `\n`); a `<script>` string is rendered as text; primary button label follows the status rule; delete button absent for reviewers; history renders "via MCP".
- `Onboarding.test.tsx`: admin sees "Create a key"; after a mocked create the secret appears in the curl block; reviewer sees "Ask an admin for the ingest key.".
- `statusCopy.test.ts`: announce sentences singular/plural and the notFound suffix.
- `shortcuts.test.ts` updated for checkbox targets.

## Acceptance

- `cd web && npm run typecheck && npm test && npm run build` pass; paste outputs.
- Against `make dev` (after `make ui`) or Vite on 5173 + `make dev`: send ~60 items with curl to `/v1/feedback` using an ingest key (create one via the UI or the API; vary userId, platform, add line breaks, one userId that is an email, one message with HTML in it), then exercise: onboarding → inbox switch, filters + URL, search, load more, keyboard j/k/x/e/a/i/n/Escape///?, bulk status and bulk tag, export CSV/JSON downloads with filters, detail (status, tags add/remove, history, delete), per-user view and delete all. Take screenshots of inbox, detail, per-user, onboarding and inbox zero; check the console has no errors and the network panel shows only same-origin requests. Report what you checked.
- No git commands of any kind.
