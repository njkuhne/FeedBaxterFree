# UX phase 1, brief A: safe and fast triage (undo, detail as the reading surface)

Author: Opus director · Milestone UX phase 1 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 1" items 1 to 3, the "Do not" list, "Decisions the reviewers asked for"), `docs/SPEC.md` Styleguide, `docs/design/m3-ui-design.md` §0 ground rules (all still apply). Defects fixed here are listed in `docs/design/ux-review/REPORT.md` "Bugs and defects found".

Front end only. No backend change, no new npm dependency. Work only in this worktree (`/Users/njkuhne/Projects/FeedBaxterFree-ux`); scratch files under `bin/`. No git commands.

## 0. Ground rules (review fails on any of these)

- Every m3-ui-design §0 rule: tokens only, Fraunces only for the wordmark and visible page `<h1>`s, feedback text as React text children only, no `any`, plain sentence-case copy, no exclamation marks, Baxter silent, no shadows. `npm test` runs `scripts/check.mjs`; it must pass.
- One primary button per screen. Keyboard model kept and extended, never reduced.
- Nothing from the verdict's "Do not" list: no split pane, no four-option segmented status control, no renamed statuses, no skeleton loaders.

## 1. Files

New:
```
web/src/feedback/listStore.ts        web/src/feedback/listStore.test.ts
web/src/feedback/undo.tsx            web/src/feedback/undo.test.tsx
web/src/feedback/origin.ts           (FromList type, parse, labels)
web/src/feedback/useKeyQueue.ts      (buffer keypresses while a request runs)
web/src/pages/FeedbackDetail.nav.test.tsx
```
Changed:
```
web/src/components/Toast.tsx (+ Toast.test.tsx)   web/src/components/PageTitle.tsx
web/src/lib/shortcuts.ts (+ test)                  web/src/lib/filters.ts (+ test)
web/src/lib/endpoints.ts                           web/src/feedback/statusCopy.ts (+ test)
web/src/feedback/useFeedbackList.ts                web/src/feedback/FeedbackList.tsx (+ test)
web/src/feedback/FeedbackRow.tsx                   web/src/feedback/ShortcutHelp.tsx
web/src/feedback/testHelpers.tsx                   web/src/pages/FeedbackDetail.tsx (+ test)
web/src/pages/Inbox.tsx   web/src/pages/PerUser.tsx   web/src/routes.tsx
web/src/styles/feedback.css   web/src/styles/components.css
```

## 2. Shared list store (`feedback/listStore.ts`)

Today the list cache is two module-level maps inside `useFeedbackList.ts`, and a status change on the detail page never reaches it. Move the cache into a small store that every mutation goes through, so the list, the detail page's Previous/Next and Undo all see the same rows.

```ts
export interface CachedList { filters: Filters; items: Feedback[]; nextCursor: string | null }
export type FeedbackEvent =
  | { type: "apply"; items: Feedback[]; reason: "change" | "undo" | "tags" }
  | { type: "remove"; ids: string[] }
  | { type: "page"; key: string };
export function listKey(filters: Filters): string;            // toApiParams(filters).toString()
export function getList(key: string): CachedList | undefined;
export function setList(key: string, list: CachedList): void;   // no event
export function getFocus(key: string): string | null;
export function setFocus(key: string, id: string): void;
export function applyFeedback(items: Feedback[], reason: "change" | "undo" | "tags"): void;
export function removeFeedback(ids: string[]): void;
export function appendPage(key: string, page: FeedbackPage): void; // dedupe by id, sets nextCursor
export function subscribe(fn: (e: FeedbackEvent) => void): () => void;
export function clearFeedbackCache(): void;                    // lists and focus; re-exported from useFeedbackList.ts
/** Pure: what a list looks like after these items changed. */
export function applyToList(list: { items: Feedback[]; nextCursor: string | null }, filters: Filters, changed: Feedback[]): { items: Feedback[]; nextCursor: string | null };
```

- `applyToList`: for each changed item, if `matchesFilters(item, filters)` it replaces the row with the same id, or, if absent, is inserted at its sorted place (order: `receivedAt` descending, then `id` descending, the API order) but only when `nextCursor === null` or the item is not older than the last loaded row (otherwise it belongs to a page not loaded yet). If it does not match, the row is removed. Idempotent.
- `applyFeedback` runs `applyToList` on every cached list, then emits `apply`. `removeFeedback` drops the ids from every cached list and emits `remove`. `appendPage` emits `page`.
- `lib/filters.ts` gains `matchesFilters(item: Feedback, f: Filters): boolean`: status (`"all"` matches any), `tag` (case-insensitive against `item.tags`), `channel` (`item.channelId`), `userId` (exact), `q` (`item.message.toLowerCase().includes(q.trim().toLowerCase())`, empty matches). Same semantics as the API.
- `useFeedbackList` keeps its reducer and public API but reads and writes the cache through the store, stores `filters` in the cache entry, and subscribes: on `apply` it dispatches an `apply` action that runs `applyToList` with its own filters; on `remove` it removes; on `page` for its own key it re-reads the cache. Its `remove`/`replace` methods stay for internal use but FeedbackList now mutates through the store.
- `endpoints.stats(filters?: Filters)` sends `tag`, `channel`, `userId` and `q` (trimmed) when non-empty, never `status`. The server ignores them until brief D; the UI is written for the honest counts now.

## 3. Toast with an action, and Undo

### Toast (`components/Toast.tsx`)

`show(message, kind = "success", options?)` where
```ts
interface ToastOptions {
  action?: { label: string; onAction: () => void };
  /** A toast with a key replaces any visible toast with the same key. */
  key?: string;
  /** Milliseconds; default 5000 (success, info), 8000 (error). */
  duration?: number;
  /** Called once when the toast leaves for any reason (timeout, dismiss, replaced, action). */
  onClose?: () => void;
  /** false: render outside the live regions because the caller announces it. Default true. */
  live?: boolean;
}
```
`useToast()` also returns `dismiss(key: string)`. The action renders as a button after the message: paper text, 14 px / 500, 1 px paper underline with 3 px offset, 44 px hit area (same `::before` trick as `.toast-dismiss`), paper focus ring (`.toast :focus-visible` already). Clicking it runs `onAction` then closes the toast. Timers pause on hover and on focus inside the toast (existing behaviour, keep it). Non-live toasts render in a third container inside `.toasts` without a role, placed above the two live regions. Existing tests keep passing.

### Undo (`feedback/undo.tsx`)

```ts
export interface UndoRecord {
  /** Visible toast text, e.g. "Marked 3 reviewed". */
  message: string;
  /** Snapshots of the items before the change, with their old status. */
  before: Feedback[];
  /** Detail only: after an auto-advance, Undo returns to the restored item if the user is still on `landedOn`. */
  revisit?: { landedOn: string; path: string; state: unknown };
}
export function UndoProvider({ children }: { children: ReactNode }): JSX.Element;
export function useUndo(): { offer: (r: UndoRecord) => void; undo: () => Promise<void>; available: boolean };
export function groupByStatus(before: Feedback[]): Map<Status, string[]>;   // pure, exported for tests
```

- `offer` keeps exactly one record (a new offer replaces the old) and shows `toast.show(message, "success", { key: "undo", duration: 6000, live: false, action: { label: "Undo", onAction: undo }, onClose })`. When the toast closes for any reason other than its own replacement by a newer offer, the record is cleared: Undo is available only while its toast is visible.
- `undo()`: take and clear the record; for each status group from `groupByStatus(before)`, one `bulkStatus(ids, previousStatus)` call (sequential; IDs that come back in `notFound` were deleted meanwhile and are skipped). Then `applyFeedback(before restricted to found ids, "undo")`, toast (`key: "undo"`, no action, default duration) "Restored 3 items." / "Restored 1 item.", and announce the same sentence. With `revisit` and `location.pathname === landedOn`, `navigate(path, { replace: true, state })`. On error: error toast with the server message; the record is gone.
- This writes a second history entry per item (verdict decision); no backend change.
- `z` is handled here, once, on `document` keydown via `shortcutFor` (action `"undo"`, so it is ignored in text fields, with modifiers and while a dialog is open). Nothing happens when no record is available.
- Provider placement: `routes.tsx` `Providers` becomes Toast → Announcer → Undo → Auth (Undo needs the router, the toast and the announcer). `testHelpers.renderPage` wraps the same way inside `MemoryRouter`.

### Status change copy (`feedback/statusCopy.ts`)

- `ACTION_LABEL`: `reviewed: "Mark reviewed"`, `actioned: "Mark actioned"`, `ignored: "Mark ignored"`, `new: "Mark new"`. `ACTION_ORDER` stays reviewed, actioned, ignored, new. Update ShortcutHelp labels to match.
- New `markedToast(count, status)`: "Marked 3 reviewed", "Marked 1 reviewed" (always with the count; parallel with the button labels).
- Announcements keep `markedSentence` / `markedOneSentence` and append " Press z to undo." Example: "Marked 3 as reviewed. Press z to undo."

### Who offers Undo

Every status change: a single-row key press or click in the list, every bulk action, and every change on the detail page. Before the request, snapshot the affected items (`before`). After success: build the changed items (the PATCH response, or `{ ...item, status }` for bulk), `applyFeedback(changed, "change")`, `removeFeedback(notFound)` if any, announce, `offer({ message: markedToast(updated, status), before })`.

## 4. List (`FeedbackList.tsx`, `FeedbackRow.tsx`, `Inbox.tsx`, `PerUser.tsx`)

- `applyStatus` follows §3. It still computes the next row to focus before the rows leave (existing logic), but rows now leave through the store event, not `list.remove`.
- After an `apply` event with `reason: "undo"` while mounted, focus the first restored row present in the list (in list order).
- Keypresses during a request are buffered, not dropped (`feedback/useKeyQueue.ts`): while a status request is in flight, list actions (`j`, `k`, `x`, `Enter`, `e`, `a`, `i`, `n`, `Escape`) are queued in order; when the request settles and the list has re-rendered, they run one by one against the new state (a queued status key starts the next request, and anything after it waits again). Queue length is capped at 10; extra keys are dropped. `z` is not queued (Undo during a request runs after it, through the provider, which waits on its own in-flight flag the same way).
- `handleListKey` must handle the new actions explicitly: `"undo"` is ignored here (the provider owns it), `"back"` (`u`) is ignored in the list. Never pass an unknown action to `applyStatus`.
- Inbox drops the `onChanged` prop and instead subscribes to the store: on any event, refresh stats (`stats(filters)` with the active filters) and, for `reason: "tags"`, refresh tags.
- Row links carry the origin in router state (§5): `state={{ from: FromList }}`. FeedbackList gets a new prop `originLabel: string` ("Inbox" on the inbox, the user ID on the per-user page) and builds `FromList` from it, its `filters`, `origin`, and the current `location.pathname + location.search`.

## 5. Detail page as the reading surface

### Origin (`feedback/origin.ts`)

```ts
export interface FromList {
  kind: "inbox" | "user";
  key: string;        // listKey(filters)
  filters: Filters;
  path: string;       // pathname + search of the list, e.g. "/?status=reviewed&tag=bug" or "/user/usr_8f2a91"
  label: string;      // "Inbox" or the user ID
}
export function readFrom(state: unknown): FromList | null;  // type-guarded; anything malformed is null
export function positionLabel(from: FromList): string;      // "in New", "in Reviewed", "in All", "from usr_8f2a91"
```

### Layout, message first

```
← Inbox                                         Previous   4 of 17 in New   Next
[● New] · usr_8f2a91 · ios · 2.3.0 · 1 Mar 2026, 09:30 (2 h ago)
The export button does nothing on Safari.               (16.5/1.65, pre-line, 33em)
1. Open reports …
[Mark reviewed]  Mark actioned   Mark ignored   Mark new     (four fixed slots)
Tags … Metadata … History … More from this user · Delete feedback
```

- No visible "Feedback" title. `PageTitle` gains `hidden?: boolean` (adds `visually-hidden`); the detail page renders `<PageTitle hidden>Feedback from usr_8f2a91</PageTitle>` ("Feedback with no user ID" when null). `usePageTitle("Feedback")` unchanged, so focus still moves to the (hidden) h1 on each navigation. This is the one approved Styleguide exception (verdict); do not edit SPEC.md, Fable records it.
- Nav line: Back on the left, Previous / position / Next on the right, one line at 1280 px (wraps below 880 px, Back first). Previous and Next are text actions with those exact words; position text ink muted 12.5 px, `aria-live` off.
- Message comes straight after the quiet meta line. Everything else as today.

### Back, Escape and `u`

- Back's visible label is `from.label` with the arrow icon; accessible name "Back to Inbox" or "Back to feedback from usr_8f2a91". No `from` (deep link): label "Inbox", goes to `/`.
- Going back: `setFocus(from.key, <current id>)`, then `navigate(-1)` when `from` exists and `location.key !== "default"`, else `navigate(from?.path ?? "/")`. This fixes Back from a detail opened on the per-user page (it went to `/`).
- `Escape` (the `clearSelection` action) and `u` (new action `back`) do the same. FeedbackList already restores focus to the cached focus id on mount.

### Previous / Next and position

- Source: `getList(from.key)`; recompute on every store event (subscribe). Position = index of the current id. Total: `items.length` when `nextCursor === null`; otherwise `stats(from.filters)` (fetched once per `from.key` and on store events): `byStatus[status]` for a status filter, `total` for All. Line: "4 of 17 in New". If the item is not in the cached list, or there is no `from`, render no position and no Previous/Next, only Back.
- Previous disabled at index 0. Next disabled on the last row when `nextCursor === null`. On the last loaded row with a cursor, Next fetches `listFeedback(from.filters, nextCursor)` (loading label "Loading…"), `appendPage`, then moves.
- Moving: `navigate("/feedback/<id>", { replace: true, state: { from } })`. Replace, so the browser's Back and our Back still return to the list.
- `j` = Next, `k` = Previous on detail (only the letters: ArrowDown/ArrowUp keep scrolling the page here).
- Seed from cache: when the cached list holds the item, render it immediately (meta, message, actions, tags) while `GET /api/feedback/{id}` loads; History shows "Loading history…" until then. No "Loading…" blank between items.

### Status actions

- Always the four actions in `ACTION_ORDER`, labels from `ACTION_LABEL`, in four fixed-width slots (`.status-actions` is a grid: `grid-template-columns: repeat(4, 9.5rem)`, gap 8 px; below 560 px `repeat(2, …)`). Buttons fill their slot, so swapping variants never moves anything.
- Primary (the one primary on the page): new → Mark reviewed, reviewed → Mark actioned, actioned and ignored → none. The others are text actions.
- The current status's own action is rendered `disabled` with its label unchanged (the chip in the meta line says the status). Never missing.
- Double submission: an in-flight ref (not only state) guards `changeStatus`; all four are disabled while a request runs. A double click sends one request.
- Keys `e` `a` `i` `n` pressed while a request runs are buffered (`useKeyQueue`) and applied after it settles, against the refreshed item; the same for `j`, `k`, `u`, Escape.

### After a decision

- `changeStatus(next)`: snapshot, `PATCH`, `applyFeedback([updated], "change")`, announce, offer Undo.
- If `from` exists and `!matchesFilters(updated, from.filters)` (the item left the list it was opened from): auto-advance, on by default. Target = the row after the current one in the list as it was before the change; if none and there is a cursor, fetch the next page first; if still none, go back to the list (as Back, focusing the row before the current one). Advance with `navigate(..., { replace: true, state: { from } })` and pass `revisit: { landedOn: "/feedback/<target>", path: "/feedback/<current>", state: { from } }` in the Undo record so Undo brings you back.
- Otherwise stay and re-fetch the item for its history.
- No auto-advance toggle in this phase (decision: Undo is the safety net; a switch belongs in the phase 3 shortcuts dialog). Note it in `docs/BACKLOG.md` under a "UX phase 3" heading.

### Delete

On success: `removeFeedback([id])`, toast "Feedback deleted.", then go back to the originating list as Back does, with the focus id set to the next row (or the previous one). Deep link: `/`.

## 6. Shortcuts (`lib/shortcuts.ts`, `ShortcutHelp.tsx`)

Add `z: "undo"` and `u: "back"` to the map and the `ShortcutAction` union. ShortcutHelp lists "z · Undo the last change" and uses the new labels ("Mark ignored", "Mark new"). Detail-only meanings (`j`/`k`, `u`, Escape) are not listed there in this phase.

## 7. Styles

Toast action and non-live container (components.css). Detail nav line, hidden title, fixed status slots (feedback.css). Tokens only. Reduced motion already handled globally.

## 8. Tests (Vitest; names are binding)

`listStore.test.ts`
- `applyToList replaces, removes and inserts in sorted order`
- `applyToList does not insert rows older than the loaded window`
- `applyFeedback updates every cached list by its own filters`

`filters.test.ts`: `matchesFilters covers status, tag, channel, userId and q`.

`shortcuts.test.ts`: `maps z to undo and u to back`.

`Toast.test.tsx`
- `a keyed toast replaces the previous one`
- `the action button runs and closes the toast`
- `pauses on hover and focus and closes after the duration`

`undo.test.tsx`: `groupByStatus groups ids by previous status`.

`FeedbackList.test.tsx`
- `shows an undo toast after a single status key and Undo restores the row in place`
- `bulk undo sends one status call per original status`
- `z undoes the last change`
- `a key pressed during a request is applied after it`
- existing tests updated for the new labels

`FeedbackDetail.test.tsx`
- `status labels and order are the same for every status` (four buttons in order, labels constant, current one disabled, at most one `.btn-primary`)
- `a double click sends one request`
- `a key pressed during a request is applied after it`
- `has no visible title and a hidden h1 naming the user`
- existing primary-button test rewritten for the new rule (no longer asserts a missing button)

`FeedbackDetail.nav.test.tsx` (render real routes with several history entries; add a `renderRoutes(entries, routes)` helper to `testHelpers.tsx` that accepts `{ pathname, state }` entries)
- `shows the position and moves with Next, Previous, j and k`
- `Next at the end of the loaded rows fetches the next page`
- `a deep link shows no position and Back goes to the inbox`
- `auto-advances after a decision that removes the item and offers Undo`
- `Undo after auto-advance returns to the restored item`
- `at the end of the list a decision returns to the list`
- `Escape returns to the list with that row focused`
- `Back from a detail opened on the per-user page returns to the per-user page`
- `delete returns to the originating list`

## 9. Acceptance

- `cd web && npm ci && npm run typecheck && npm test && npm run build` all pass; paste the tail of each (test count).
- No change outside the files in §1 (plus `docs/BACKLOG.md` for the one note). No git commands.
- Report: anything in this brief you could not do as written, and why, rather than working around it silently.
