# UX phase 1, brief B: rows that can be read, one compact header, a bulk bar that does not move

Author: Opus director · Milestone UX phase 1 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 1" items 4 and 6, the "Do not" list), `docs/SPEC.md` Styleguide, `docs/design/m3-ui-design.md` §0 (ground rules) and §3. Defects from `docs/design/ux-review/REPORT.md` "Bugs and defects found" fixed here: bulk bar shift, bulk bar wrapping, no-op "Mark new" on the New tab, current-row marker lost, stale relative times, tag pills and meta text on selected rows, New dot on selected rows, Ignored chip border.

Brief A is done and binding: the list store (`feedback/listStore.ts`), Undo (`feedback/undo.tsx`, every status change offers Undo with `before` snapshots), `useKeyQueue`, `FromList` origin, the new labels in `statusCopy.ts` ("Mark reviewed", "Mark actioned", "Mark ignored", "Mark new"). Build on them; do not re-implement. Brief C comes next and will replace the bulk bar's tag field with a shared `TagEditor` and make search span all statuses; leave clean seams for it, do not build it.

Front end only, no new npm dependency. Work only in `/Users/njkuhne/Projects/FeedBaxterFree-ux`; scratch under `bin/`. No git commands.

## 0. Ground rules (review fails on any of these)

m3-ui-design §0 in full (tokens only, Inter, text children only, no `any`, plain copy, no shadows, one primary button per screen, `scripts/check.mjs` passes). WCAG 2.1 AA: 44 px targets, every control labelled (visually hidden labels are fine and are required where the visible label goes), status always marker plus a label available to assistive technology. Nothing from the "Do not" list: no sort menu, no multi-tag filter, no platform/version/date filters, no day-group headers, no saved views, no bulk delete, no skeleton loaders.

## 1. Files

New:
```
web/src/feedback/Toolbar.tsx          (search, Tag, Channel, Export on one line)
web/src/feedback/ExportMenu.tsx       (replaces ExportLinks.tsx; delete ExportLinks.tsx)
web/src/feedback/ListHeader.tsx       (select-all + summary + filter chips, or the bulk bar, same line)
web/src/feedback/useNow.ts            (ticking clock for relative times)
web/src/feedback/selectMatching.ts    (page IDs client-side up to 500)
web/src/feedback/Toolbar.test.tsx  web/src/feedback/ListHeader.test.tsx  web/src/feedback/selectMatching.test.ts
```
Changed:
```
web/src/feedback/FeedbackRow.tsx (+ test)   web/src/feedback/FeedbackList.tsx (+ test)
web/src/feedback/BulkBar.tsx                web/src/feedback/FilterBar.tsx (tabs only now, or fold into Inbox; your call, say which)
web/src/pages/Inbox.tsx   web/src/pages/PerUser.tsx   web/src/pages/FeedbackDetail.tsx (platform names, ticking time only)
web/src/lib/format.ts (+ test)   web/src/lib/shortcuts.ts (+ test)   web/src/components/icons.tsx
web/src/styles/feedback.css   web/src/styles/components.css
```

## 2. Rows (`FeedbackRow.tsx`)

```
[☐] ●  The export button does nothing on Safari and I have tried     2 h ago
       twice. Steps: open reports, press export…
       usr_8f2a91 · iOS · 2.4.1 · (bug) (export)
```

- **Preview:** `previewOf(message)` now keeps line breaks: trim, collapse runs of spaces and tabs to one space, collapse three or more consecutive line breaks to two, cut at 300 characters. Rendered with `white-space: pre-line` and a two-line clamp (`display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden`). 15 px ink as today. The link's accessible name stays the preview text.
- **Whole row is the click target:** remove the `.row-link::after` overlay (it blocks text selection and will block the tag-pill links of brief C). Instead the `<li>` gets an `onClick` that navigates to the item exactly like the link (same router state), unless the click started inside an interactive element (`a`, `button`, `input`, `label`, `select`) or the user has selected text (`window.getSelection()?.toString()` non-empty). The row shows `cursor: pointer`. The link stays the keyboard and assistive-technology path and keeps Cmd/Ctrl-click for a new tab.
- **Meta line by context**, via new props `showStatus`, `showChannel`, `showUser` computed in FeedbackList:
  - status word: shown only when the list's status filter is `all` (All tab, per-user page; brief C's search results run on All). On single-status tabs the word is dropped; the marker stays and gets an accessible label: `StatusMarker` gains an optional `label` prop that renders `role="img" aria-label="New"` instead of `aria-hidden`. Use it whenever the word is not shown.
  - channel: only when there are two or more channels and no channel filter is active.
  - user ID: not on the per-user page (`origin === "user"`).
  - platform: `platformName(p)` in `lib/format.ts`: `ios` → iOS, `macos` → macOS, `ipados` → iPadOS, `web` → Web, `android` → Android (case-insensitive match), anything else shown exactly as sent. Also used on the detail page's meta line.
  - then app version, then up to 3 tag pills and "+N" as today.
- **Current row marker:** the current row (roving focus) gets class `row-current` and the 3 px dusk deep left bar whether or not focus is inside the list. Remove the `:focus-within` rule; the focus ring on the link stays.
- **Relative times refresh:** `useNow(intervalMs = 60_000): number` returns `Date.now()` and re-renders every interval while `document.visibilityState === "visible"` (and once on becoming visible). FeedbackList calls it once and passes `now` to rows (`relativeTime(iso, now)`); the detail page uses it for its "(2 h ago)".
- **Contrast on selected rows** (selected = dusk tint background): meta and time in ink soft (already), tag pills switch to card stock fill (`.row-selected .pill`), the New dot switches to dusk deep (`.row-selected .marker-new`; 3.15:1 → above 4.5:1; decision, the dot keeps its shape). Ignored chip border: dashed `var(--bark)` instead of kraft (1.8:1 → 5.3:1). Hairline between selected rows: `.row-selected + .row-selected` gets a `var(--kraft)` top border so adjacent selected rows stay distinct.

## 3. Header: tabs, then one toolbar line

```
Inbox
New 12   Reviewed 30   Actioned 10   Ignored 3   All 55
[🔍 Search messages              ×]  [All tags ▾]  [All channels ▾]  [Export ▾]
[☐] 17 items · tag bug × · "safari" × · Clear all                     ← list header line
─────────────────────────────────────────────────────────────────────
rows
```

- Page head: the h1 alone (Export moves into the toolbar). Tabs: the existing `Segmented` status control, unchanged behaviour and counts.
- **Toolbar** (`Toolbar.tsx`), one line at 1280 px and at 880 px, `display: flex; gap: 8px; align-items: center`; it may wrap below 880 px (phase 2 handles mobile):
  - Search: `TextField` with visually hidden label "Search", `type="search"`, placeholder "Search messages", a magnifier icon inside on the left (decorative, `aria-hidden`, `pointer-events: none`, padding-left 36 px), and a clear button inside on the right, shown only when there is text: `aria-label="Clear search"`, `XIcon`, 44 px hit area; clicking clears the text immediately (no debounce), keeps focus in the field. Hide the native WebKit search cancel button (`::-webkit-search-cancel-button { appearance: none; }`). It takes the remaining width (`flex: 1; min-width: 12rem`). `/` and Escape behave as today.
  - Tag: `SelectField` with `hideLabel` label "Tag", compact (`width: auto`, max 12rem), options "All tags" then each tag as "bug (12)" from `/api/tags` counts. Unknown current tag kept as today.
  - Channel: same pattern, label "Channel", "All channels" then "Web (23)" with counts from the unfiltered `byChannel` of the first stats load; shown only with two or more channels.
  - Export (`ExportMenu.tsx`): a text-action button "Export" with the download icon and a chevron, `aria-expanded`, `aria-controls`; opens a small panel below it (card stock, kraft border, radius 6, padding 12, right-aligned to the button, z-index above rows; not a modal, not `role="menu"`, a disclosure). Panel content: one line of ink soft text saying what will be exported, then two links (`<a href={exportUrl(...)} download>`) "CSV" and "JSON", each 44 px tall. The sentence: "Export 17 items" plus a description of the active filters joined by ", ": status label (omit on All), "tag bug", "channel Web", "matching "safari"", for example "Export 17 items · New, tag bug". The count is the active tab's count from stats (honest once brief D lands); while stats are unknown: "Export all matching items · New, tag bug". Escape closes and returns focus to the button; clicking outside closes; choosing a link closes. Per-user page: the same ExportMenu with "from usr_…" in the description.
- **List header line** (`ListHeader.tsx`): directly above the rows, a fixed-height line (`min-height: 44px`, same padding in both states, bottom hairline). Two states that swap in place, never both, never adding a line:
  - Idle: the select-all checkbox (bare: label visually hidden, "Select all on this page"; indeterminate when some are selected), then the summary in ink soft 13 px: "17 items" (the active tab's count; "Showing 50" style fallback "50 loaded" when unknown), then one removable chip per active non-status filter: "tag bug", "channel Web", ""safari"" (search text in typographic quotes). Chips: oat pill with ink soft text and a remove button (`aria-label="Remove filter tag bug"`, XIcon, 44 px hit area via `::before`); then a text action "Clear all" when any chip exists (clears tag, channel and search, keeps the status tab). Removing a chip updates the URL like the corresponding control.
  - Selection: the bulk bar (below), same height, same position.
  - The per-user page uses the same line with no chips.

## 4. Bulk bar (`BulkBar.tsx`, rendered by ListHeader)

```
[☑] 17 selected   Mark reviewed   Mark actioned   Mark ignored   Tag…   Remove tag…       Clear
```

- `role="toolbar"` `aria-label="Bulk actions"`, one line at 1280 px (no wrap; at the 760 px content width this must fit: text actions with padding 0 4, gap 12, 14 px). The select-all checkbox stays at the start of the line (checked or indeterminate) so the line keeps its left edge. No sticky panel, no border box, no margin: it is the list header line.
- Status actions in `ACTION_ORDER`, labels from `ACTION_LABEL`, except the one equal to the current tab's status (on the New tab there is no "Mark new"; on All all four show). A status action applies to the selection exactly as brief A (one request, Undo offered with snapshots).
- "Tag…": opens a small popover anchored to it (same panel style as ExportMenu) with a `TextField` "Tag" (visible label), a `<datalist>` of existing tags, and a text action "Add tag"; Enter adds; Escape closes and returns focus. Brief C replaces the inside with `TagEditor`; keep this popover's content in one small component (`BulkTagPopover`) so it can be swapped.
- "Remove tag…": shown only when the selection has at least one tag; opens a popover listing only the tags present on the selected items, each as a text action "bug (3)" (count = selected items carrying it); choosing one calls `removeTags(ids, [name])`, applies through the store (`applyFeedback(..., "tags")`), announces "Removed tag bug from 3.", closes.
- "Clear" (text action, right-aligned): clears the selection. Escape still clears it.
- While a request runs, actions are disabled and the count reads "Updating 17…" (as today).
- **Select all matching:** when every loaded row is selected and the list has a next cursor, a one-line notice appears directly under the header line (this is the only time a line is added, and only after the user selected a full page): "All 50 on this page are selected. " + text action "Select all 230 matching" (N = the active tab's count; when N > 500: "Select the first 500 matching"; unknown N: "Select all matching, up to 500"). Choosing it pages IDs client-side with `selectMatching(filters, alreadyLoaded, cap = 500)`: calls `listFeedback(filters, cursor, 200)` until no cursor or 500 items, returns the items (it does not add rows to the list). The notice then reads "All 230 matching are selected." or "The first 500 matching are selected. Bulk actions handle up to 500 at a time." with a "Clear selection" text action. Status and tag actions then use all selected IDs; Undo snapshots come from the fetched items. While paging: "Selecting…".
- **Range selection:** Shift-click on a row checkbox selects (or deselects, matching the anchor row's new state) every loaded row between the anchor (the last row toggled by click or `x`) and the clicked row. `Shift+x` (key `"X"`) does the same from the anchor to the current row: add `X: "rangeSelect"` to the shortcut map; queue it like `x`. ShortcutHelp lists "Shift+x · Select a range".

## 5. Tests (Vitest; names are binding)

`FeedbackRow.test.tsx`
- `shows the status word only on All`
- `the marker has an accessible label when the word is hidden`
- `hides the channel with one channel or a channel filter`
- `hides the user ID on the per-user page`
- `shows platform display names`
- `keeps line breaks in the two-line preview`
- `a click anywhere on the row opens it, but not on the checkbox`
- `marks the current row without focus`

`format.test.ts`: `platformName maps known platforms and keeps unknown ones`.

`Toolbar.test.tsx`
- `renders search, tag, channel and export on one toolbar line` (a single toolbar container holds all four controls, labels present but visually hidden)
- `the clear button empties the search`
- `export says what it will export`

`ListHeader.test.tsx`
- `shows removable filter chips and Clear all`
- `the bulk bar replaces the header line in the same element` (the same header element stays mounted, with the same class, and contains either the summary or the toolbar, never both)
- `hides the action for the current tab`
- `remove tag lists only tags on the selection`

`selectMatching.test.ts`: `pages ids up to the 500 cap`, `stops when the cursor runs out`.

`FeedbackList.test.tsx`
- `select all matching selects up to 500 and bulk actions use them`
- `shift-click selects a range`
- `Shift+x selects a range from the anchor to the current row`
- `relative times refresh` (fake timers, advance 60 s, the text changes)

## 6. Acceptance

- `cd web && npm ci && npm run typecheck && npm test && npm run build` pass; paste the tails (with test count).
- Only the files in §1 change. No git commands.
- Report every deviation from this brief and why. Say whether you kept FilterBar or folded it.
