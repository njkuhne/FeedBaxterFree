# UX phase 2: the mobile shell

Author: Opus director · Milestone UX phase 2 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 2" item 9, the "Do not" list, "Decisions the reviewers asked for": mobile is a supported reading and triage surface, bulk stays desktop, no swipe gestures), `docs/SPEC.md` (Admin UI, Styleguide, Privacy), `docs/design/m3-ui-design.md` §0 (ground rules). Phase 1 (briefs `ux1-a` to `ux1-d`) is done and binding: the toolbar, list header, bulk bar, `TagEditor`, Undo, list store, detail navigation and the keyboard model. Build on them; do not re-implement them.

Front end only. No new npm dependency, no Go change. Work only in `/Users/njkuhne/Projects/FeedBaxter/free-ux2`; scratch files under its `bin/`. No git commands.

## The problem

At 375 px the wrapped rail plus the account line take 200 to 400 px; the status tabs wrap to two lines; Tag, Channel and Search stack as full-width fields; the first feedback row sits near the bottom of the first screen. The detail page repeats the whole navigation block above the message, and its meta line orphans "·" separators when it wraps.

## 0. Ground rules (review fails on any of these)

- m3-ui-design §0 in full: tokens only (no raw colours outside `styles/tokens.css`), Inter for UI, Fraunces only for the wordmark and page titles, no `any`, plain sentence-case copy, no shadows on cards, `scripts/check.mjs` passes.
- **Desktop (≥ 880 px) does not change**, except the meta-line markup in §6, which must look the same as today (dots between parts).
- One primary button per screen. On detail it is the status action that is primary today (`Mark reviewed` for New, `Mark actioned` for Reviewed); on mobile it lives in the sticky bar and nowhere else.
- WCAG 2.1 AA: 44 px targets for everything this brief adds, visible focus rings, every control labelled, statuses keep icon plus label, contrast from the existing tokens only.
- Nothing from the "Do not" list: no swipe gestures, no split pane, no dark mode, no new filters, no bulk delete, no skeletons. Light mode only.
- The keyboard model is unchanged on every width (j/k, x, Shift+x, Enter, Escape, the status keys, `t`, `z`, `/`, `?`, `u`).
- `prefers-reduced-motion`: any animation added here is wrapped in `@media (prefers-reduced-motion: no-preference)` (the global rule in `base.css` stays as the backstop).

## 1. Files

New:
```
web/src/lib/useMediaQuery.ts          useMediaQuery(query) and useNarrow() (max-width: 879px)
web/src/lib/useMediaQuery.test.ts
web/src/shell/TopBar.tsx               compact bar + Menu disclosure (below 880 px)
web/src/shell/TopBar.test.tsx
web/src/components/MetaList.tsx        flex-wrap list with CSS-only separators (§6)
web/src/components/MetaList.test.tsx
web/src/test-media.ts                  mockMatchMedia(narrow: boolean) helper for tests
```
Changed:
```
web/index.html                         viewport-fit=cover (so env(safe-area-inset-*) works)
web/src/shell/Shell.tsx                TopBar below 880 px, Rail at 880 px and up
web/src/components/Segmented.tsx       scroll container, active option scrolled into view
web/src/feedback/Toolbar.tsx (+ test)  Filters button and disclosure on narrow
web/src/feedback/ExportMenu.tsx        extract ExportPanel (description + CSV/JSON links)
web/src/feedback/ListHeader.tsx (+ test)  Select mode on narrow
web/src/feedback/BulkBar.tsx           hideClear prop
web/src/feedback/FeedbackList.tsx (+ test)  narrow, selectMode, showChecks
web/src/feedback/FeedbackRow.tsx (+ test)   time in meta line on narrow, MetaList, showCheck
web/src/pages/FeedbackDetail.tsx (+ test)   sticky action bar on narrow, MetaList
web/src/styles/layout.css  feedback.css  components.css  tokens.css (one size token)
```
If a file outside this list needs a change, say which and why in your report.

## 2. Breakpoint hook (`lib/useMediaQuery.ts`)

```ts
export function useMediaQuery(query: string): boolean   // useSyncExternalStore over window.matchMedia
export const NARROW_QUERY = "(max-width: 879px)";
export function useNarrow(): boolean                    // useMediaQuery(NARROW_QUERY)
```
- Subscribe with `mql.addEventListener("change", …)`. When `window.matchMedia` is missing (jsdom), return `false`: every existing test keeps running as desktop with no change.
- Call `useNarrow()` once per page-level component (Shell, Toolbar, FeedbackList, FeedbackDetail) and pass `narrow` down as a prop; do not call it in every row.
- CSS keeps the same breakpoint (`max-width: 879px`) for layout; JS only decides what is rendered (TopBar vs Rail, checkboxes, the disclosure, the sticky bar).

`test-media.ts`: `mockMatchMedia(narrow: boolean)` installs a `window.matchMedia` stub whose `matches` is `narrow` for `NARROW_QUERY` (and false for anything else), with `addEventListener`/`removeEventListener` no-ops; returns a restore function. Tests call it in `beforeEach` and restore in `afterEach`.

## 3. Compact top bar (`shell/TopBar.tsx`)

```
┌──────────────────────────────────────────┐  56 px
│ [baxter] FeedBaxter                [Menu ▾]│
└──────────────────────────────────────────┘
  open:
┌──────────────────────────────────────────┐
│ Inbox                                     │  44 px rows, aria-current on the current one
│ Channels and keys      (admins only)      │
│ People                 (admins only)      │
│ Settings               (admins only)      │
│ Profile                                   │
│ ───────────────────────────────────────── │
│ nick@example.com                          │  account line: email (ellipsis) and role
│ Admin                                     │
│ Sign out                                  │
└──────────────────────────────────────────┘
```
- `Shell` renders `<TopBar />` when `useNarrow()` is true, otherwise `<Rail />`. Only one of them is in the DOM. Move the nav item list and `isCurrent` out of `Rail.tsx` into a shared export (same file is fine) so both use the same items and the same current-page rule. Delete the now-dead `@media (max-width: 879px)` rail rules in `layout.css`.
- Markup: `<header className="topbar">` holding the brand link (`BaxterSmall size={28} decorative` plus `<span className="wordmark">FeedBaxter</span>`, Fraunces 24 px as today) and the Menu button; the panel is `<nav aria-label="Main" id={panelId} className="topbar-panel" hidden={!open}>`.
- Menu button: `Button variant="text"` with the visible word **Menu** plus `ChevronDownIcon` (rotated 180° while open), `aria-expanded`, `aria-controls={panelId}`, min 44 × 44 px. No icon-only button.
- Panel contents in this order: Inbox, then for admins only Channels and keys, People, Settings; then Profile; then a hairline; then the account line (email in ink with ellipsis, `title` = email; role "Admin" or "Reviewer" in ink muted); then Sign out (`Button variant="text"`). Links are 44 px rows like `.rail-link`, `aria-current="page"` styled as on the rail.
- Opening moves focus to the first link in the panel. Escape (anywhere in the bar or panel) closes and returns focus to the Menu button. A `pointerdown` outside the header closes it without moving focus. Navigation closes it (effect on `pathname`). Focus leaving the header (a `focusout` whose `relatedTarget` is outside) closes it.
- The panel overlays the content (`position: absolute; top: 100%; left: 0; right: 0; z-index: 20`), card stock background, kraft bottom border, padding `var(--s-2) var(--s-4) var(--s-4)`. It does not push the page down.
- Bar: `height: 56px` (`min-height`, so text zoom can grow it), `padding: 0 var(--s-4)`, paper background, hairline bottom border, `position: relative`. Not sticky (screen space goes to feedback).
- Motion: under `prefers-reduced-motion: no-preference`, the panel fades in with a 4 px downward slide over 150 ms; nothing on close.
- Same bar on every authenticated page, including detail and per-user. Nothing else repeats above the content.

## 4. Status tabs: one scrolling row (`Segmented.tsx`, `components.css`)

- Below 880 px the segmented control is one row: `flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none` plus `::-webkit-scrollbar { display: none }`; options `flex: none; white-space: nowrap`. Desktop keeps `flex-wrap: wrap`. Counts stay.
- If the `fieldset` misbehaves as a scroll container (it does in some engines), wrap it in `<div className="segmented-scroll">` and scroll that instead; the radiogroup semantics stay on the fieldset.
- Keyboard: the options are radios, so Tab lands on the checked one and the arrow keys move between them; the browser scrolls the focused option into view. No extra `tabindex` on the container.
- On mount and when `value` changes, scroll the checked option into view horizontally only: set the container's `scrollLeft` so the option is fully visible (compute from `offsetLeft`/`offsetWidth`); do not call `scrollIntoView` (it can scroll the page vertically). Guard for jsdom (no layout: widths are 0, do nothing harmful).
- Below 880 px reduce option padding to `0 var(--s-2)` so more tabs fit; the bottom kraft rule spans the full width.

## 5. Filters (`Toolbar.tsx`, `ExportMenu.tsx`)

Narrow layout:
```
[🔍 Search messages              ×] [Filters · 1 ▾]
┌ disclosure (in flow, under the toolbar) ───────┐
│ Tag      [All tags                    ▾]        │
│ Channel  [All channels                ▾]        │  (only with 2+ channels, as today)
│ Export 17 items · New, tag bug                  │
│ [CSV] [JSON]                                    │
└─────────────────────────────────────────────────┘
 17 items  (tag bug ×)  Clear all          ← the phase 1 chips, unchanged
```
- Toolbar takes a `narrow` prop (Inbox passes `useNarrow()`). Desktop renders exactly what it renders today.
- Narrow: search box stays as today (magnifier, placeholder "Search messages", clear ×, `flex: 1`, `min-width: 0` so it shrinks at 320 px), followed on the same line by a Filters button: `Button variant="text"`, visible text `Filters` with no active filter, `Filters · N` with N active (N counts Tag and Channel only; search is already visible), `ChevronDownIcon`, `aria-expanded`, `aria-controls`, 44 px.
- The disclosure is an in-flow `<div id=… className="filters-panel" hidden={!open}>` directly under the toolbar line (not an absolute popover): Tag and Channel as full-width `SelectField`s with **visible** labels, then the export block. It stays open while the user changes selects; it closes on the Filters button or Escape inside it (focus back to the button). Opening does not move focus (it is in flow, the next Tab reaches it).
- Extract `ExportPanel({ filters, count, channelName, onDone? })` from `ExportMenu`: the description paragraph plus the CSV and JSON links. `ExportMenu` keeps using it inside its popover (desktop and per-user unchanged); the narrow disclosure renders `ExportPanel` inline under a small "Export" label (`--fs-label`, ink muted). The CSV and JSON links get 44 px height on narrow.
- `.popover-note` loses `white-space: nowrap` below 880 px, and `.popover` gets `max-width: calc(100vw - 2 * var(--s-4))` so the per-user page's Export popover cannot overflow at 320 px.

## 6. Meta lines without orphaned separators (`components/MetaList.tsx`)

Both the detail meta line and the row meta line become a list whose separators are not text:
```tsx
<MetaList className="detail-meta" label?>
  <li>…</li><li>…</li>
</MetaList>
// renders <ul class="meta-list {className}"> children </ul>
```
CSS (one place, `components.css`):
```css
.meta-list {
  --meta-gap: var(--s-4);
  display: flex; flex-wrap: wrap; align-items: center;
  gap: var(--s-1) var(--meta-gap);
  list-style: none; margin: 0;
  padding-left: var(--s-1); margin-left: calc(var(--s-1) * -1);   /* room for the first item's focus ring */
  clip-path: inset(-12px -12px -12px 0);                           /* clips only the left edge */
}
.meta-list > li { position: relative; min-width: 0; overflow-wrap: anywhere; }
.meta-list > li + li::before {
  content: "·" / "";                       /* empty alternative text: not read */
  position: absolute; left: calc(var(--meta-gap) / -2); translate: -50% 0;
  color: var(--ink-muted);
}
```
Every item after the first draws its dot centred in the gap before it. The first item of a wrapped line sits at the list's left edge, so its dot falls left of the clip and disappears: no line ever starts or ends with a dot, and no dot is a text node. If you find a cleaner technique with the same two properties (no separator text nodes, no orphan at either end of a wrapped line), use it and say so.

- **Detail**: status chip, user ID link (or "No user ID"), platform, version, channel (2+ channels), time (absolute `<time>` plus "(relative)"). Delete `MetaPart` and the literal `·` spans.
- **Rows**: status word (when shown), user, platform, version, channel, then the tag pills as the last `li`; replace the `Fragment`/`" · "` construction. On narrow, the relative `<time>` is the last item in the meta line (after the tags) instead of the grid's own column; on desktop it stays on the right exactly as today. Render it in one place or the other, never both.

## 7. Rows and Select mode (`FeedbackRow.tsx`, `FeedbackList.tsx`, `ListHeader.tsx`, `BulkBar.tsx`)

- `FeedbackList` calls `useNarrow()` and holds `selectMode` (boolean, false on mount and on every query change, alongside the existing resets). `showChecks = !narrow || selectMode`. Any selection change on narrow (an `x` press, a checkbox) sets `selectMode` true, so the keyboard model still works.
- `FeedbackRow` gets `showCheck` and `narrow`. Without a check, the row grid is `16px minmax(0, 1fr)` (marker, body); with it, `var(--target) 16px minmax(0, 1fr)` on narrow. The message clamp stays two lines. Row `min-height: var(--target)` on narrow (today's 64 px stays on desktop); vertical padding `var(--s-3)`.
- `ListHeader` gets `narrow`, `selectMode`, `onSelectMode(on: boolean)`:
  - Narrow, not in Select mode: no select-all checkbox; the summary and chips as today, then a `Select` text button at the end of the line (44 px, `margin-left: auto`).
  - Narrow, in Select mode: the select-all checkbox first; then either "Select items" (ink soft, while none are selected) or the `BulkBar`; then a `Done` text button that clears the selection and leaves Select mode. `BulkBar` gets `hideClear` so it does not also show Clear.
  - Narrow: the "Select all N matching" offer is never shown (the `notice` of kind `offer` is null). Bulk stays desktop-first.
  - Escape in Select mode clears the selection and leaves Select mode. Focus after `Select`: the first row's checkbox; after `Done`: the `Select` button.
  - Desktop: unchanged (checkbox always, no Select or Done buttons).
- Bulk bar on narrow wraps: `.bulk { flex-wrap: wrap; white-space: normal; row-gap: var(--s-1) }` and `.list-head { flex-wrap: wrap }` at `max-width: 879px`; the bulk actions keep 44 px height. Its Tag… and Remove tag… popovers anchor to the header's left edge on narrow (`.list-head { position: relative }`, `.list-head .popover-wrap { position: static }`, popover `left: 0; right: 0`), so they never overflow.

## 8. Detail page on narrow (`FeedbackDetail.tsx`)

```
 ← Inbox          Previous  4 of 17 in New  Next       (unchanged, top)
 ● New  usr_8f2a91  iOS  2.4.1  12 Oct 2026, 14:02 (2 h ago)    (MetaList)
 The export button does nothing on Safari …                      (message)
 Tags · Metadata · History · More from this user · Delete
 ─────────────── sticky bar, fixed to the bottom ───────────────
 [Mark reviewed] [Mark actioned]  [ Next ]
 [Mark ignored ] [Mark new     ]  [      ]
```
- The page calls `useNarrow()`. There is still exactly **one** set of status buttons: on narrow the existing `.status-actions` block renders as the sticky bar (class `status-actions status-bar`) at the end of `.detail`, with a Next button added; on desktop it stays inline under the message, unchanged.
- Order and labels exactly as desktop (`ACTION_ORDER`, `ACTION_LABEL`), the current status disabled, the primary as today. `role="group"` with `aria-label="Status"`.
- Next: shown when `hasNext` (same rule and handler as the top pager, including `loading={paging}` and `disabled={queue.busy}`). Hidden when there is no next item; then the four actions use the full width.
- Layout: `position: fixed; left: 0; right: 0; bottom: 0; z-index: 15`; card stock background, kraft top border; padding `var(--s-2) var(--s-4)` plus `padding-bottom: calc(var(--s-2) + env(safe-area-inset-bottom))`; grid `grid-template-columns: 1fr 1fr auto` with the four actions in two rows of two and Next in the third column spanning both rows; buttons 44 px tall, `padding: 0 var(--s-2)`, labels never truncated or wrapped. Verify the arithmetic at 320 px (288 px usable: two action columns plus Next must fit without wrapping a label); if it cannot, Next takes a third row across the full width and you say so.
- Add `--bottom-bar-height: 112px` to `tokens.css` (two 44 px rows, an 8 px gap, 8 px padding top and bottom). On narrow, `.detail` gets `padding-bottom: calc(var(--bottom-bar-height) + env(safe-area-inset-bottom) + var(--s-6))` so nothing hides under the bar, and toasts sit above it: `body:has(.status-bar) .toasts { bottom: calc(var(--bottom-bar-height) + env(safe-area-inset-bottom) + var(--s-3)); }`.
- `web/index.html`: viewport becomes `width=device-width, initial-scale=1, viewport-fit=cover`.
- The top `detail-nav` (Back, Previous, position, Next) stays. On narrow it may wrap to two lines; that is fine.

## 9. Reflow at 320 px (every page)

No horizontal page scroll at 320 px on inbox, detail, per-user, channels, people, settings, profile, sign-in and change-password. Known risks to fix:
- `.page-title`: `overflow-wrap: anywhere` (per-user "Feedback from usr_…").
- Long IDs and code: `overflow-wrap: anywhere` on `.mono`, `code`, `.code-block` (or the CodeBlock's class), key prefixes, token values, metadata `dd`, history items, the user ID link in the meta line. `pre` blocks scroll inside themselves (`overflow-x: auto`), never the page.
- Tables already sit in `.table-scroll`; leave that.
- `.content` padding on narrow: `var(--s-4)` all round (was `var(--s-6) var(--s-4)`); `.page-title` margin-bottom `var(--s-4)` on narrow.
- `.search-box` `min-width: 12rem` becomes `min-width: 0` on narrow.
- Popovers capped as in §5.

## 10. Tests (Vitest, jsdom; these names exactly)

`lib/useMediaQuery.test.ts`
- `useNarrow is false without matchMedia`
- `useNarrow follows the mocked media query`

`shell/TopBar.test.tsx` (with `mockMatchMedia(true)` where it renders through `Shell`)
- `Menu button opens the panel and moves focus to the first link`
- `Escape closes the panel and returns focus to the Menu button`
- `a click outside closes the panel`
- `navigating closes the panel`
- `admin links are hidden for reviewers`
- `admin links are shown for admins`
- `shell renders the rail on desktop and the top bar on narrow`

`feedback/Toolbar.test.tsx`
- `narrow toolbar shows search and a Filters button, not the selects`
- `Filters button shows the active count`
- `Filters button toggles the disclosure with aria-expanded`
- `the disclosure holds Tag, Channel and Export`
- `desktop toolbar is unchanged` (Tag select, Export button present, no Filters button)

`feedback/ListHeader.test.tsx` / `FeedbackList.test.tsx`
- `narrow list hides row checkboxes and select-all outside Select mode`
- `Select shows the checkboxes and select-all`
- `selecting rows in Select mode shows the bulk bar`
- `Done clears the selection and leaves Select mode`
- `x on narrow enters Select mode`
- `select all matching is not offered on narrow`

`feedback/FeedbackRow.test.tsx`
- `narrow row puts the time in the meta line`
- `row meta has no separator text`

`components/MetaList.test.tsx`
- `meta list renders items with no separator text nodes` (walk every text node: none is `·` or whitespace-plus-`·`)

`pages/FeedbackDetail.test.tsx`
- `narrow detail renders the sticky bar with the four actions in order`
- `the current status is disabled in the sticky bar`
- `the sticky bar shows Next when a next item exists`
- `the sticky bar has no Next without a next item`
- `the sticky bar holds the only primary button`
- `detail meta has no separator text`

All existing tests stay green unchanged (they run as desktop because jsdom has no `matchMedia`). If one has to change, say which and why.

## 11. Acceptance (the director runs these)

`cd web && npm ci && npm run typecheck && npm test && npm run build`; `make vet`; `make test`. Browser check at 375 px and 320 px: inbox, open Menu, open Filters, Select mode with two rows selected, detail with the sticky bar, per-user, channels, people, settings, profile, sign-in; `document.documentElement.scrollWidth <= window.innerWidth` on each.

## 12. Report

Files added and changed, test count before and after, every deviation from this brief with the reason, anything you could not verify without a browser.
