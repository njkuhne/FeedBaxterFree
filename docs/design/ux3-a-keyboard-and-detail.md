# UX phase 3, brief A: keyboard, detail page, per-user page

Author: Opus director · Milestone UX phase 3 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 3" items 10 and 11, the "Do not" list, "Decisions the reviewers asked for"), `docs/SPEC.md` (Admin UI, Styleguide, Privacy), `docs/BACKLOG.md` section "UX phase 3" (the auto-advance switch and the single-key switch land here), `docs/design/m3-ui-design.md` §0. Phases 1 and 2 (`ux1-a` to `ux1-d`, `ux2-mobile`) are done and binding: Toolbar, ListHeader, BulkBar, TagEditor, Undo, list store, detail navigation, key queue, TopBar, MetaList, ExportMenu. Build on them; do not re-implement them.

Front end only. No new npm dependency, no Go change. Work only in `/Users/njkuhne/Projects/FeedBaxter/free-ux3`; scratch files under its `bin/`. No git commands.

## 0. Ground rules (review fails on any of these)

- m3-ui-design §0 in full: tokens only (no raw colours outside `styles/tokens.css`), Inter for UI, Fraunces only for the wordmark and page titles, no `any`, plain sentence-case copy, no shadows on cards, `scripts/check.mjs` passes.
- One primary button per screen. WCAG 2.1 AA: 44 px targets for everything added, visible focus rings, every control labelled, statuses keep icon plus label.
- Feedback text and metadata are plain text. Never `dangerouslySetInnerHTML`, never Markdown. Line breaks preserved.
- Nothing from the "Do not" list (no split pane, no notes, no saved views, no bulk delete, no sort menu, no new filters, no skeletons, no Baxter speech).
- Every phase 1 and 2 behaviour keeps working and every existing test keeps passing (adjust a test only where this brief changes the behaviour it pins, and say which in your report).
- `localStorage` access is always wrapped in try/catch; the app works with it missing or throwing.

## 1. Files

New:
```
web/src/lib/prefs.ts (+ prefs.test.ts)              the two persisted switches
web/src/feedback/shortcuts.tsx (+ shortcuts.test.tsx) ShortcutsProvider, useShortcuts, the dialog
web/src/components/Kbd.tsx                          the faint key hint
web/src/feedback/historyCopy.ts (+ test)            history sentences
web/src/feedback/userSummary.ts (+ test)            the per-user summary line
```
Changed:
```
web/src/lib/shortcuts.ts (+ test)        r alias, Shift+J/K, singleKeys context
web/src/routes.tsx                       ShortcutsProvider inside the providers
web/src/feedback/ShortcutHelp.tsx        deleted (the dialog replaces it)
web/src/feedback/FeedbackList.tsx (+ test)  no inline help; Shift+j/k; prefs
web/src/feedback/BulkBar.tsx             key hints
web/src/components/Toast.tsx (+ test)    optional key hint on the action
web/src/feedback/undo.tsx                passes the z hint
web/src/shell/Rail.tsx                   "Keyboard shortcuts" button
web/src/pages/FeedbackDetail.tsx (+ tests)  copy actions, metadata, history, more-from-user, Data removal, advance pref, hints
web/src/pages/PerUser.tsx (+ new PerUser.test.tsx)  summary, status tabs, delete section and done state
web/src/styles/*.css                     as needed, tokens only
```
If a file outside this list needs a change, say which and why in your report.

## 2. Preferences (`lib/prefs.ts`)

```ts
export interface Prefs { advance: boolean; singleKeys: boolean }
export const DEFAULT_PREFS: Prefs = { advance: true, singleKeys: true };
export function getPrefs(): Prefs
export function setPref<K extends keyof Prefs>(key: K, value: Prefs[K]): void
export function usePrefs(): Prefs   // useSyncExternalStore; updates every subscriber in this tab, and on the "storage" event from other tabs
```
- Storage keys `fbx.pref.advance` and `fbx.pref.singleKeys`, values `"1"` / `"0"`. Anything else, or storage that throws, reads as the default. A failed write still updates the in-memory value for this tab.

## 3. Shortcut mapping (`lib/shortcuts.ts`)

- Add `r: "reviewed"` (alias of `e`).
- Add actions `"extendNext"` and `"extendPrev"` for `J` and `K` (Shift+j / Shift+k arrive as `key === "J"` / `"K"`).
- `ShortcutContext` gains `singleKeys: boolean`. When it is false, every action whose key is a single printable character returns null **except `?`**: letters, `/`, `x`, `X`, `J`, `K`, `r`, `z`, `u`, `t` all stop. Arrow keys, Enter and Escape keep working (they are not character keys under WCAG 2.1.4). Ctrl/Meta/Alt combinations are still ignored as today, so browser shortcuts are untouched.
- Every caller passes `singleKeys` from `getPrefs()` read at key time (not a stale render value): FeedbackList, FeedbackDetail, undo.tsx (`z`), and the shortcuts provider (`?`).

## 4. The shortcuts dialog (`feedback/shortcuts.tsx`)

`ShortcutsProvider` (mounted in `routes.tsx` inside `UndoProvider`, around `AuthProvider`) holds the open state and exposes `useShortcuts(): { open: () => void }`.
- A document `keydown` listener opens the dialog on `?` (through `shortcutFor`, so typing in a field never opens it, and not while another dialog is open). It works on every signed-in page, including detail (today detail ignores `?`).
- `FeedbackList` no longer handles `toggleHelp` and no longer renders `ShortcutHelp`; delete `ShortcutHelp.tsx`. `?` while the dialog is open does nothing extra; Escape closes it (the Dialog already does) and focus returns to the opener.
- Rendered with the existing `Dialog` (title "Keyboard shortcuts"), with a Close text button at the end.

Content, in this order. Each group is an `h3` plus a `dl` (`dt` holds one or more `<kbd>`, `dd` the meaning). Copy exactly:

| Group | Keys → meaning |
| --- | --- |
| List | `j` or `↓` → Next item · `k` or `↑` → Previous item · `Enter` → Open the item · `/` → Search |
| Status (list and detail) | `e` or `r` → Mark reviewed: read, still open · `a` → Mark actioned: something was done about it · `i` → Mark ignored · `n` → Mark new: reopen it · `t` → Add a tag |
| Detail | `j` → Next item · `k` → Previous item · `u` or `Escape` → Back to the list |
| Selection | `x` → Select or deselect the item · `Shift`+`x` or Shift+click → Select a range · `Shift`+`j` / `Shift`+`k` → Select the next or previous item too · `Escape` → Clear the selection |
| Undo | `z` → Undo the last change, while its notice is showing |
| Help | `?` → Show this list |

Below the groups, a "Settings" `h3` with two switches, each an `<input type="checkbox" role="switch">` via the existing `Checkbox` (pass `role="switch"`; extend the component only if the type needs it):
- "Advance to the next item after a decision" (pref `advance`). Hint under it, ink muted: "On the detail page, after you mark an item and it leaves the list."
- "Single-key shortcuts" (pref `singleKeys`). Hint: "Turn off if you use speech input or press keys by accident. Arrow keys, Enter, Escape and ? keep working."
Changes apply at once and persist (§2).

Entry points to the dialog:
- `?` anywhere (above).
- Rail (≥ 880 px): a quiet text button "Keyboard shortcuts" in `.rail-account`, above "Sign out". It calls `open()`.
- Not in the narrow TopBar menu (phones have no keyboard).
- The old link below the last row is gone.

## 5. Key hints (`components/Kbd.tsx`)

`<Kbd k="e" />` renders `<kbd className="kbd-hint" aria-hidden="true">e</kbd>`: Inter `--fs-label`, `--ink-muted`, 1 px `--hairline` border, `--radius-button`, padding 0 4px, margin-left `--s-1`. Faint, never bold.
- On the status buttons in the BulkBar (`e`, `a`, `i`, `n`) and on the detail page's inline status actions (same keys), plus the toast's Undo action (`z`).
- The button also gets `aria-keyshortcuts` with the key (so screen readers get it from the attribute, not the hidden text). Button names stay exactly "Mark reviewed" etc.
- Hidden when `singleKeys` is off (not rendered, and no `aria-keyshortcuts`), and not rendered when `narrow` (the sticky mobile bar and the narrow bulk bar get no hints).
- Toast: `ToastOptions.action` gains optional `hint?: string`; `ToastView` renders `<Kbd>` inside the action button and sets `aria-keyshortcuts`. `undo.tsx` passes `hint: "z"` only when `singleKeys` is on and the viewport is not narrow (use `window.matchMedia(NARROW_QUERY)` guarded for jsdom, or pass through from `useNarrow` in the provider).

## 6. Selection with Shift+j / Shift+k (FeedbackList)

- `extendNext`: if the current row is not selected, select it; then move to the next row (loading the next page exactly as `j` does on the last row) and select that row too. Set the range anchor to the row it started from if no anchor exists. `extendPrev` is the same upwards.
- It only ever adds to the selection. Shift+click and Shift+x keep today's anchor behaviour.
- On narrow screens the first extend enters Select mode (as `x` does today).
- On the detail page `J`/`K` do nothing.

## 7. Detail page (`pages/FeedbackDetail.tsx`)

**Advance switch.** When `advance` is off, a decision that takes the item out of its list keeps you on the item: refetch it, offer Undo without `revisit`, focus stays where it was. When on, today's behaviour (move to the next item, or back to the list when none) is unchanged.

**Copy actions.** A quiet row of two text buttons, "Copy message" and "Copy link", directly after the tags section (so the status actions keep their position under the message). Use `copyText` from `lib/clipboard.ts`.
- Copy message copies `f.message` exactly (line breaks kept). Success toast "Message copied." Failure toast (error): "Could not copy. Select the text and copy it manually."
- Copy link copies `${window.location.origin}/feedback/${encodeURIComponent(f.id)}`. Success toast "Link copied."

**Metadata.** Keep the `dl.facts`. Value rendering by type:
- string → as is, in a `dd` with `white-space: pre-line`;
- number, boolean, null → `String(v)`;
- object or array → `<pre className="meta-json">{JSON.stringify(v, null, 2)}</pre>` inside the `dd`: `--font-mono`, `--fs-meta`, `--ink-soft`, `white-space: pre-wrap`, `overflow-wrap: anywhere`, no background box. React text only.

**History as sentences** (`feedback/historyCopy.ts`):
```ts
export function historyTime(iso: string): string      // "1 Nov 2026 15:57" (dateOnly + " " + HH:MM, local)
export function receivedSentence(iso: string): string  // "Received 28 Oct 2026 09:12"
export function historySentence(h: HistoryEntry): string
// "Marked reviewed by admin@localhost, 1 Nov 2026 15:57"
// "Marked actioned by ai@example.com via MCP, 1 Nov 2026 16:03"   (" via MCP" only when via === "mcp")
// "Marked new by a removed user, 2 Nov 2026 08:00"                 (userEmail null)
```
The status word is the lowercase status (`reviewed`, `actioned`, `ignored`, `new`). Each `li` wraps the time in `<time dateTime={iso}>` (render the sentence as text parts so the time is the `time` element). Oldest first, as today.

**More from this user.** When `f.userId !== null`, request `stats({ ...defaultFilters, status: "all", userId: f.userId })` once per item id. `others = total - 1`.
- `others > 0`: link "More from this user (3)" to `/user/<id>`.
- `others === 0`: no link.
- Unknown (loading or failed): the link without a count.

**Data removal section** (admins only), last on the page, after the footer link:
```
<section aria-labelledby=…>
  <h2 class="section-title">Data removal</h2>
  <p class="soft">Deleting is permanent. Deleted feedback still counts toward this month's limit.</p>
  <Button variant="danger">Delete feedback</Button>
</section>
```
The existing confirmation dialog and its behaviour stay. "Delete feedback" is no longer in the same row as "More from this user".

## 8. Per-user page (`pages/PerUser.tsx`)

**Status tabs.** The `Segmented` used by the inbox, with All first here: All, New, Reviewed, Actioned, Ignored, counts from `stats({ userId })` (refreshed on store events, like the inbox). The status lives in the URL (`?status=new`; absent means all) so Back from detail returns to the same tab. Changing the tab changes `filters.status` for the `FeedbackList`.

**Summary line** (`feedback/userSummary.ts`), a `p.muted` directly under the title:
```ts
export function userSummary(items: Feedback[], total: number | null, complete: boolean, now: Date): string
// "2 items · first 28 Oct · latest today · Web 2.4.1, iOS 3.1.0"
```
- Count: `total` (from the stats All count) when known, else `items.length`.
- "first <date>" from the oldest loaded item, only when `complete` (every item loaded); omitted otherwise.
- "latest <day>": "today", "yesterday", else "28 Oct" (year added when not this year).
- Versions: distinct `platformName(platform) + " " + appVersion` (or either part alone; skip items with neither), newest first, at most 3, then "+N more".
- Computed from the All list's rows for this user: read them from the list store (`getList(listKey(allFilters))`, re-read on `subscribe`), independent of the active tab. If the All list is not in the store (the user opened another tab first), fetch its first page once with `listFeedback`.
- Parts joined with " · ". One line, no Baxter.

**Delete all, in its own section** (admins only), below the list:
```
<section aria-labelledby=…>
  <h2 class="section-title">Data removal</h2>
  <p class="soft">Use this when your customer asks you to remove this person's data. It permanently deletes every item from {userId}.</p>
  <Button variant="danger">Delete all feedback for this user</Button>
</section>
```
- Confirmation title with the count from stats: "Delete all 4 feedback items from usr_8f2a91?" ("Delete the 1 feedback item from …?" for one; "Delete all feedback from …?" while the count is unknown). Body unchanged.
- After deletion, the page replaces the tabs, list and section with: `<p role="status">Deleted 4 feedback items from usr_8f2a91.</p>` and a link "Back to the inbox" (to `/`). Focus moves to that paragraph (`tabIndex={-1}`). The success toast is dropped (the page says it). `clearFeedbackCache()` stays.
- The Export menu stays in the page head; it exports the active tab's filters.

## 9. Tests (by name; all must exist and pass)

`lib/prefs.test.ts`
- `defaults to advance on and single keys on`
- `persists a switch and reads it back`
- `falls back to defaults when storage throws`
- `notifies subscribers when a pref changes`

`lib/shortcuts.test.ts` (add)
- `r marks reviewed like e`
- `Shift+j and Shift+k extend the selection`
- `with single keys off, letters and slash do nothing`
- `with single keys off, arrows, Enter, Escape and ? still work`

`feedback/shortcuts.test.tsx`
- `? opens the shortcuts dialog from the inbox`
- `? opens the shortcuts dialog from the detail page`
- `? typed in the search box does not open the dialog`
- `lists every group with its keys`
- `the advance switch persists`
- `the single-key switch turns letter shortcuts off`
- `the rail button opens the dialog`

`feedback/FeedbackList.test.tsx` (add)
- `Shift+j selects the current and the next row`
- `Shift+k selects upwards`
- `status buttons in the bulk bar show key hints`
- `key hints are hidden when single keys are off`
- `no shortcuts link below the rows`

`components/Toast.test.tsx` (add)
- `an action with a hint shows the key and sets aria-keyshortcuts`

`feedback/historyCopy.test.ts`
- `writes a UI change as a sentence`
- `adds via MCP only for MCP changes`
- `names a removed user`

`pages/FeedbackDetail.test.tsx` / `.nav.test.tsx` (add)
- `Copy message copies the plain text with line breaks`
- `Copy link copies the item URL`
- `nested metadata is pretty-printed as text`
- `metadata never renders HTML` (a value like `"<b>x</b>"` shows literally)
- `history lines are sentences`
- `More from this user shows the count of other items`
- `More from this user is hidden when there are no other items`
- `Delete feedback sits in a Data removal section`
- `with advance off, a decision stays on the item`
- `status actions show key hints on desktop and not on narrow`

`feedback/userSummary.test.ts`
- `summarises count, first, latest and versions`
- `omits first while the list is incomplete`
- `caps versions at three`

`pages/PerUser.test.tsx`
- `shows the summary line under the title`
- `status tabs filter the list and keep the status in the URL`
- `delete all sits in its own section with a sentence`
- `the confirmation names the count`
- `after deletion the page says what happened and links back`

## 10. Done means

`cd web && npm ci && npm run typecheck && npm test && npm run build` green. Report: files changed, test count before and after, any test you changed and why, any deviation from this brief and why.
