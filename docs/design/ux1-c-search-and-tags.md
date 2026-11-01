# UX phase 1, brief C: search that finds things, one tag editor

Author: Opus director · Milestone UX phase 1 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 1" items 7 and 8, the "Do not" list, "Decisions the reviewers asked for": search with a query spans all statuses and shows that it does), `docs/SPEC.md` Styleguide and Privacy (feedback text is plain text, never HTML), `docs/design/m3-ui-design.md` §0. Defects fixed here (REPORT.md "Bugs and defects found"): search silently scoped to the current tab, "Loading feedback…" flash on each search change, native `<datalist>` inconsistent in Safari/VoiceOver, tag × targets under 44 px, "Nothing matches these filters" when only a tab is set, Baxter on the filtered-empty state.

Briefs A and B are done and binding: list store and `applyFeedback`, Undo provider (`feedback/undo.tsx`), `useKeyQueue`, `FromList`, `Toolbar`, `ListHeader` with filter chips, `BulkBar` with `BulkTagPopover` (to be replaced here) and `RemoveTagPopover`, `usePopover` (exported from `ExportMenu.tsx`), rows with `showStatus`. Build on them.

Front end only, no new npm dependency. Work only in `/Users/njkuhne/Projects/FeedBaxterFree-ux`; scratch under `bin/`. No git commands.

## 0. Ground rules

m3-ui-design §0 in full; `scripts/check.mjs` passes. Feedback text, tag names and search text are only ever React text children: highlighting splits a plain string into text and `<mark>` elements, never builds HTML. No `dangerouslySetInnerHTML`, no regex built from user input without escaping (prefer `indexOf` on lowercased strings). WCAG 2.1 AA, 44 px targets. Nothing from the "Do not" list (no multi-tag filter, no saved views, no skeleton loaders).

## 1. Files

New:
```
web/src/components/Highlight.tsx        (+ Highlight.test.tsx)
web/src/feedback/TagEditor.tsx          (+ TagEditor.test.tsx)
web/src/feedback/emptyCopy.ts           (+ emptyCopy.test.ts)  filtered-empty sentences
web/src/pages/Inbox.test.tsx            (search behaviour at page level)
```
Changed:
```
web/src/pages/Inbox.tsx   web/src/pages/FeedbackDetail.tsx (+ test)   web/src/pages/PerUser.tsx
web/src/feedback/FeedbackList.tsx (+ test)   web/src/feedback/FeedbackRow.tsx (+ test)
web/src/feedback/BulkBar.tsx   web/src/feedback/ListHeader.tsx   web/src/feedback/useFeedbackList.ts
web/src/feedback/undo.tsx (+ test)   web/src/feedback/statusCopy.ts   web/src/feedback/ShortcutHelp.tsx
web/src/components/TagPill.tsx   web/src/components/EmptyState.tsx   web/src/lib/shortcuts.ts (+ test)
web/src/styles/feedback.css   web/src/styles/components.css
```

## 2. Search

### Search spans all statuses, and the tab strip shows it (decision)

Of the two options in the milestone (switch the tabs to All visibly, or a "2 more in Reviewed, Actioned" link under the results), we switch to All. Reason: once brief D lands, the tab counts are computed under the query, so with All selected the strip itself reads "New 2 · Reviewed 1 · Actioned 0 · Ignored 0 · All 3": it carries the "more in other statuses" information without a second element, it is one list rather than a list plus a pointer to another, and the user can narrow to a status with one click.

- When the search text goes from empty to non-empty (typing or paste, after the 300 ms debounce, or the URL gaining `q`), the URL is written with `status=all` in the same `setParams` call, and Inbox remembers the status that was active (component state, `statusBeforeSearch`). The All tab shows as selected.
- While a query is active, the user may click any tab to narrow the search; that is respected and not overridden by later typing (the switch happens only on the empty → non-empty transition).
- When the search is cleared (clear button, chip removal, deleting the text, "Clear all"), the status returns to `statusBeforeSearch` if the user did not pick a tab meanwhile; otherwise it stays as picked. A deep link with `q` and no remembered status stays on whatever the URL says.
- Rows show the status word on All (brief B rule), so results from different statuses are distinguishable.

### Highlight (`components/Highlight.tsx`)

```ts
export function Highlight({ text, query }: { text: string; query: string }): JSX.Element;
export function splitMatches(text: string, query: string): { text: string; match: boolean }[]; // pure, exported for tests
```
- Case-insensitive, every occurrence, using `text.toLowerCase()` and `query.trim().toLowerCase()` with `indexOf`. If `text.toLowerCase().length !== text.length` (rare Unicode case-mapping changes), return the text unhighlighted. Empty query: one plain segment.
- Renders a fragment of strings and `<mark className="hl">` elements. Style: `background: var(--oat); color: inherit; border-bottom: 1px solid var(--bark)` (the underline is the non-colour cue); on selected rows `background: var(--card-stock)`. Contrast of ink and ink soft on oat and card stock passes.
- Used in row previews when the list has a query, and in the detail message when the item was opened from a list with a query (`from.filters.q`). `white-space: pre-line` is unaffected (marks are inline).
- Preview windowing: if the first match starts after character 120 of the preview source, the preview starts at the word boundary about 40 characters before the match, prefixed with "…", so the match is visible inside the two-line clamp. Build this from the full message (not the 300-character cut) in `FeedbackRow`.

### Pasted user ID

When the query is a single token (no white space, 1 to 128 characters) and the list comes back empty, check `listFeedback({ ...defaultFilters, status: "all", userId: q }, null, 1)`. If that has an item, the empty state offers a text action link "Show feedback from usr_8f2a91" to `/user/<encoded q>` (exact `userId` filter, the per-user page). If not, the plain filtered-empty state. One request per query; ignore stale answers.

### No flash while searching

- Inbox stops remounting `FeedbackList` per filter (remove the `key`); `useFeedbackList` already switches keys. While a new key loads without a cache entry, keep showing the previous key's rows, dimmed (`.rows[aria-busy="true"] { opacity: 0.6 }` and `aria-busy="true"` on the list), and do not render "Loading feedback…". The list header summary reads "Searching…" (or "Loading…" when the change was not the search text) only after the request has been running for 300 ms. Selection, the current row and the select-all notice reset on every filter change, as they did on remount. First page load with nothing to show keeps "Loading feedback…", also delayed 300 ms.
- The detail page's restored focus logic (Back) must keep working; the list's mount-only effects that depended on remounting need to run on key change too. Say in your report how you handled it.

### Empty states (`feedback/emptyCopy.ts`, `components/EmptyState.tsx`)

- `EmptyState` gains `mark?: boolean` (default true). Baxter shows only for inbox zero (and onboarding, unchanged); the filtered-empty state passes `mark={false}`.
- `emptyCopy(filters, channelName?)` returns `{ heading, body }`:
  - Only a status tab, no other filter: heading "No reviewed feedback" (status word lowercased; All: "No feedback"), body "Feedback you mark reviewed appears here." (actioned: "…mark actioned…", ignored: "…mark ignored…", All: "Feedback appears here as it arrives."). New keeps the existing inbox-zero copy and Baxter.
  - With filters: heading "No matches", body naming them in one sentence: "Nothing in New tagged bug in channel Web matching "safari"." (omit absent parts; All: "Nothing tagged bug matching "safari"."). Action "Clear filters" (clears tag, channel and search, keeps the tab).

## 3. Tags

### `TagEditor` (`feedback/TagEditor.tsx`)

```ts
interface TagEditorProps {
  /** Tags on the item (detail). Omit in add-only mode (bulk). */
  tags?: string[];
  suggestions: TagCount[];          // from /api/tags
  onAdd: (name: string) => Promise<boolean>;
  onRemove?: (name: string) => void;
  /** Start with the input open (bulk popover). */
  initiallyOpen?: boolean;
  ref?: Ref<TagEditorHandle>;
}
export interface TagEditorHandle { open: () => void }
```
- Pills (`TagPill` with `onRemove`) then a text action "+ Add tag". Removable pills: height 32, the × button 32 × 32 visible with a 44 × 44 hit area (`::before`); gap between pills 12 px so neighbouring hit areas do not cover each other's ×.
- "+ Add tag" (or `open()`) swaps itself for a small input (width 14rem, 36 px tall inside a 44 px row, visually hidden label "Add tag", placeholder "Tag name") with an ARIA 1.2 combobox: `role="combobox"`, `aria-expanded`, `aria-controls` → a `role="listbox"` below, `aria-autocomplete="list"`, `aria-activedescendant` for the active option. Options: existing tags not already on the item whose name contains the typed text (case-insensitive), sorted by count descending then name, at most 8, each rendered "bug" with its count in ink muted ("12"), `role="option"`, `aria-selected` on the active one; the option's accessible name is "bug, 12 items". With empty text, the 8 most used.
- Keys in the input: Down/Up move the active option (wrapping, none active initially); Enter adds the active option's name, or the typed text when no option is active; Escape closes the listbox if open, else closes the input and returns focus to "+ Add tag" (Escape must not bubble to the page shortcuts: `stopPropagation`); Tab closes the listbox and leaves normally. Clicking an option adds it (`onMouseDown` preventDefault so the input keeps focus). After a successful add the input stays open and empty, focus stays in it. Validation: 1 to 64 characters after trim, else the error "Tags are 1 to 64 characters." under the input (`aria-invalid`, `aria-describedby`). Adding a tag the item already has (case-insensitive) just clears the input.
- No `<datalist>` anywhere after this brief.

### Where it is used

- **Detail:** the Tags section keeps its `<h2>` but visually hidden ("Tags"); the section shows only the editor (pills + "+ Add tag") directly under the status actions. Add and remove go through the API and `applyFeedback(..., "tags")` as today.
- **Bulk bar:** `BulkTagPopover`'s inside becomes `<TagEditor initiallyOpen suggestions onAdd />` (no `tags`, add-only). After an add the popover stays open for another tag; Escape in the empty, listbox-closed input closes the popover (focus back to "Tag…").
- **Rows:** tag pills become links that filter by the tag: in the inbox, `?tag=<name>` keeping status, channel and search (push); on the per-user page, `/?status=all&tag=<name>`. Accessible name "Filter by tag bug". Their `tabIndex` follows the row's roving tab index (0 only on the current row). Row click-through already ignores clicks on links. Pill style unchanged (oat, ink soft), hover underline, focus ring.

### `t` shortcut

Add `t: "tag"` to the shortcut map and union. On detail: opens the editor input (`open()`), and is ignored while it is already open (focus is in the input). In the list: with a selection, opens the bulk "Tag…" popover; without one, selects the current row first, then opens it. Queue `t` like other list keys. ShortcutHelp lists "t · Add a tag".

### Undo on tag removal

- `UndoRecord` becomes a union; the status variant is unchanged. Add `{ kind: "tag-remove"; message: string; tag: string; before: Feedback[] }` (`before` = the items that carried the tag, as they were). Undo: `addTags(ids, [tag])`, then `applyFeedback(before, "undo")`, toast and announce "Restored tag bug on 3 items." / "…on 1 item.". The status variant keeps `kind?: "status"` optional so brief A callers compile unchanged.
- Offered on detail tag removal ("Removed tag bug", announce "Removed tag bug. Press z to undo.") and on bulk Remove tag ("Removed tag bug from 3"). `z` and the toast's Undo work as for status changes.

## 4. Tests (Vitest; names are binding)

`Inbox.test.tsx`
- `a query searches all statuses and selects All` (typing a query sends a list request without `status`, the All radio is checked)
- `clearing the search returns to the previous tab`
- `a pasted user ID with no message hits offers that user's feedback`
- `keeps the previous results dimmed while searching` (no "Loading feedback…" text; rows still present with `aria-busy`)
- `the filtered-empty state names the filters and shows no Baxter`

`Highlight.test.tsx`
- `splitMatches finds every case-insensitive match`
- `an injected tag in a message renders literally` (message `"Click <b>here</b> now"`, query `here`: the container has no `b` element, `textContent` equals the message, and one `mark` with text `here`)

`FeedbackRow.test.tsx`: `highlights the match in the preview`, `tag pills link to the tag filter`.

`FeedbackDetail.test.tsx`: `highlights the query in the message when opened from a search`, `t opens the tag editor`, `removing a tag offers Undo and Undo restores it`.

`TagEditor.test.tsx`
- `adds a tag with Enter and keeps the input open`
- `arrow keys choose a suggestion sorted by use`
- `Escape closes the input and returns focus to Add tag`
- `rejects an empty or too long tag`
- `remove buttons are labelled and at least 44 px` (the computed/declared hit area: assert the class that carries the 44 px `::before`, and the accessible name "Remove tag bug")

`FeedbackList.test.tsx`: `t opens the bulk tag editor for the selection`, `bulk remove tag offers Undo`.

`emptyCopy.test.ts`: `names status-only and filtered empties`.

`shortcuts.test.ts`: `maps t to tag`.

## 5. Acceptance

- `cd web && npm ci && npm run typecheck && npm test && npm run build` pass; paste the tails with the test count.
- Only files in §1 change (if another file must change, say which and why). No git commands.
- Report every deviation and how you handled the remount-dependent effects (§2, no flash).
