# UX phase 3, brief B: inbox freshness, export errors, admin tidy, onboarding, contrast

Author: Opus director · Milestone UX phase 3 · Sources (binding): `docs/design/ux-review/VERDICT.md` ("Do, Phase 3" items 12 to 15, the "Do not" list, "Decisions"), `docs/SPEC.md` (Admin UI, Styleguide, Privacy), `docs/BACKLOG.md` "UX phase 3" (export error path), `docs/MCP.md` (the config forms), `docs/design/m3-ui-design.md` §0. Phases 1 and 2 and brief A (`ux3-a-keyboard-and-detail.md`, already built) are binding: build on them.

Front end only. No new npm dependency, no Go change. Brief C adds `lastUsedAt` to keys and `lastReceivedAt` to channels and the export `Content-Disposition` filename; build against those fields now (they may be null or absent until C lands: treat absent as null). Work only in `/Users/njkuhne/Projects/FeedBaxter/free-ux3`; scratch files under its `bin/`. No git commands.

## 0. Ground rules (review fails on any of these)

- m3-ui-design §0 in full (tokens only, Inter for UI, Fraunces only for wordmark and page titles, no `any`, sentence case, no card shadows, `scripts/check.mjs` passes).
- One primary button per screen. WCAG 2.1 AA: 44 px targets, focus rings, labels, statuses with icon plus label. Brick only on destructive actions.
- Nothing from the "Do not" list: no date presets or date filters, no dashboards or charts, no skeletons, no Baxter speech or motion beyond §8's reduced-motion-safe dot (which is not Baxter).
- Polling only while the tab is visible; never move or insert rows under the user's cursor or focus.
- Every phase 1, 2 and brief A behaviour keeps working; adjust an existing test only where this brief changes the behaviour it pins, and say which.

## 1. Files

New:
```
web/src/shell/NewCount.tsx (+ NewCount.test.tsx)   NewCountProvider, useNewCount: the app-wide New count
web/src/feedback/NewPill.tsx                       the "N new · Show" pill
web/src/lib/download.ts (+ download.test.ts)       fetch a file and save it with the server's filename
web/src/admin/McpConfig.tsx (+ McpConfig.test.tsx) the ready-to-paste config after a new token
web/src/pages/System.tsx (renamed from Settings.tsx) + System.test.tsx
web/src/lib/versionText.ts (+ test)                plain-words version line
```
Changed:
```
web/src/routes.tsx                      /system, /settings → /system redirect
web/src/shell/Shell.tsx, Rail.tsx, TopBar.tsx (+ test), usePageTitle.ts
web/src/pages/Inbox.tsx (+ test), feedback/FeedbackList.tsx (+ test), feedback/emptyCopy.ts (+ test)
web/src/feedback/ExportMenu.tsx
web/src/admin/OpsGrid.tsx (+ test)
web/src/pages/Profile.tsx
web/src/admin/PeopleTable.tsx (+ test)
web/src/pages/Channels.tsx, admin/ChannelPanel.tsx (+ test), admin/KeyTable.tsx
web/src/pages/Onboarding.tsx (+ test)
web/src/lib/types.ts                    Key.lastUsedAt, Channel.lastReceivedAt (both string | null)
web/src/styles/*.css, tokens.css        §10
```
If a file outside this list needs a change, say which and why in your report.

## 2. App-wide New count (`shell/NewCount.tsx`)

`NewCountProvider` wraps the signed-in shell (inside `Shell`, around rail/top bar and `main`). It fetches unfiltered `stats()` on mount, then every 30 s while `document.visibilityState === "visible"` (reuse `admin/usePolling`, interval 30 000), and again on every list-store event (`subscribe` from `feedback/listStore`). It exposes `useNewCount(): number | null` (`stats.byStatus.new ?? 0`; null before the first answer or after a failure with no earlier value).
- **Rail and TopBar menu:** the Inbox link reads "Inbox" plus, when the count is above 0, a count in a `span.nav-count` (ink muted, `--fs-meta`, tabular numbers): visible "Inbox 17", accessible name "Inbox, 17 new" (put ", 17 new" in visually hidden text, and the visible number `aria-hidden`).
- **Document title:** every signed-in page title becomes `(17) <title> · FeedBaxter` when the count is above 0, else `<title> · FeedBaxter` as today. Do it in `usePageTitle` by reading the count from context (outside the provider the count is null and nothing changes, so Login and Change password are untouched). Updating the prefix must not move focus (only a pathname change moves focus, as today).

## 3. Inbox freshness

**"N new · Show" pill** (`feedback/NewPill.tsx`, used by `Inbox.tsx`):
- The inbox polls `stats(filters)` every 30 s while visible (it already has `refreshStats`; drive it with `usePolling`). Keep a `baseline` total: the `stats.total` that matched the current list load (set when the inbox boots, when the filters change and after the pill reloads). On a list-store event (the user's own changes), set `baseline = min(baseline, fresh.total)` so the user's own deletions never look like arrivals.
- `arrived = max(0, latest.total - baseline)`. When `arrived > 0` and the tab is New or All, show the pill above the list header line: a quiet button "3 new · Show" (`btn btn-text` look inside a pill: card stock fill, kraft border, `--radius-pill`, 44 px tall). On Reviewed, Actioned and Ignored tabs no pill is shown (arrivals land in New; the New tab count still updates).
- Show reloads the list (`list.reload` via a new `reloadKey` prop or an imperative handle on `FeedbackList`; pick the smaller change), resets the baseline to `latest.total`, and keeps focus on the list header area (move focus to the first row only if focus was on the pill, which disappears). Announce "Showing 3 new items." politely.
- Rows are never inserted by the poll itself.
- Tab counts update from the poll (they already come from the same stats).

**Relative times:** `useNow` already refreshes them every minute while visible. Verify only; add test `relative times refresh after a minute` if none exists.

**"Showing 50 of 312":** under the list when more pages exist, replace "Showing 50" with "Showing 50 of 312" using the `count` prop when known (else "Showing 50" as today).

**Auto-load on scroll:** an `IntersectionObserver` on the load-more block calls `loadMore()` when it comes within 400 px of the viewport (`rootMargin: "0px 0px 400px 0px"`), at most one request at a time, never while `stale` or on error. The "Load more" button stays as the fallback (keyboard, no observer). Guard for `typeof IntersectionObserver === "undefined"` (jsdom): then nothing auto-loads.

**Inbox zero with reviewed waiting:** on the New tab with no other filters and an empty list, when `stats.byStatus.reviewed > 0`, the EmptyState (Baxter stays, as today) gets an action: a text button "Show 4 reviewed" that switches to the Reviewed tab, and the body becomes "4 reviewed items are still open." (singular: "1 reviewed item is still open."). With 0 reviewed, today's copy.

**Filtered empty** already names the filters (`emptyCopy`); verify, no change.

## 4. Export (`feedback/ExportMenu.tsx`, `lib/download.ts`)

```ts
export async function downloadFile(url: string): Promise<void>   // throws on failure
export function filenameFrom(contentDisposition: string | null, fallback: string): string
```
- `downloadFile` uses `fetch(url, { credentials: "same-origin" })`. Non-2xx or a network error throws. On success: `await res.blob()`, `URL.createObjectURL`, a temporary `<a download={name}>` clicked, then revoke the URL on the next tick.
- `filenameFrom` reads `filename="…"` (and `filename*=UTF-8''…` if present, preferred) from the header; otherwise the fallback `feedbaxter-export.csv|json`.
- The CSV and JSON links in `ExportPanel` become buttons. While one runs: that button shows "Exporting…" (Button `loading`) and both are disabled. On failure the panel stays open and shows `<p role="alert" class="form-error">Export failed. Try again.</p>` under the buttons; the next attempt clears it. On success the panel closes (`onDone`) as today.
- The narrow Filters panel uses the same `ExportPanel`, so it gets the same behaviour.

## 5. System page (was Settings)

- Route `/system` renders `System` (title "System"); `/settings` is `<Navigate to="/system" replace />` (admin-only like before). Nav label "System" in `navItems`.
- `OpsGrid`: drop the Version cell from the grid. Under the grid, a footer line in `p.muted` (`lib/versionText.ts`):
```ts
export function versionText(ops: Pick<Ops, "version" | "mode" | "dialect">): string
// "FeedBaxter 0.4.0 · single process · SQLite"
// "FeedBaxter development build (0.4.0-3-gab12cd-dirty) · single process · SQLite"   version containing "-dirty"
// "FeedBaxter development build · API process · Postgres"                              version "dev" or ""
```
  A leading `v` is dropped. Modes: `all` → "single process", `api` → "API process", anything else → `<mode> mode`.
- "Could not be stored": when `ops.queue.dlq > 0`, the cell's value is preceded by `AlertCircleIcon` (ink, `aria-hidden`), and the existing note becomes one line: "N submissions could not be stored. They are kept aside; see "Signals" in OPS.md to inspect and replay them." No brick anywhere on this page. At 0, no icon and no note.

## 6. Rail, Menu and Profile

- Rail: a visible "Profile" link in `.rail-account` (above "Keyboard shortcuts"), `aria-current="page"` on `/profile`. The email stays as plain text (no longer the profile link). TopBar Menu already has Profile: keep.
- **MCP config after a new token** (`admin/McpConfig.tsx`): when a token was just created, below `SecretOnce` and inside the same once-only state (it disappears when SecretOnce's Done is pressed), show a section "Connect your AI client" with two `CodeBlock`s (each has Copy):
  - "Claude Code": `claude mcp add --transport http feedbaxter ${origin}/mcp --header "Authorization: Bearer ${secret}"`
  - "JSON config (Claude Desktop and others)": the JSON form from `docs/MCP.md` with `url` `${origin}/mcp` and the header filled, 2-space indented.
  - `origin` is `window.location.origin`. One plain sentence above: "Paste one of these into your client. It includes the token, so keep it private."

## 7. People and Channels

**People** (`admin/PeopleTable.tsx`):
- The last admin's role cell is plain text "Admin" with a `span.muted` note under it: "Only admin. Make someone else an admin first." (no disabled select).
- Changing a role opens a `ConfirmDialog` before saving: title "Make jo@example.com an admin?" / "Make jo@example.com a reviewer?"; body for admin: "Admins can delete feedback and manage channels, keys and people." for reviewer: "Reviewers can read, triage, tag and export, but cannot delete or manage." (for yourself becoming a reviewer, add " You will lose access to admin pages."). Confirm label "Change role". Cancel leaves the select on the old value.
- `mustChangePassword` shows "Pending first sign-in" instead of "Temporary password".

**Channels** (`pages/Channels.tsx`, `admin/ChannelPanel.tsx`, `admin/KeyTable.tsx`):
- The always-visible create form becomes a quiet text button "New channel" (with a `+` glyph only if an icon already exists; otherwise text). Pressing it reveals the existing field and a "Create" text button plus "Cancel"; the field gets focus; Escape or Cancel hides it and returns focus to "New channel". No primary button on this page (the page has no single primary action).
- Each channel's title line: name, then `span.muted` "312 items · last received 2 h ago" (count from `stats().byChannel` loaded once with the page, `lastReceivedAt` from the channel through `relativeTime`); "No feedback yet" when the count is 0 or `lastReceivedAt` is null. Keep "created <date>" after it.
- Key table: a "Last used" column after Created: `relativeTime(lastUsedAt)` or "Never". Cell `title` attribute with the absolute time.
- "Delete channel" moves out of the header row to the end of the panel, after the key table, in its own row (`.channel-danger`, separated by a hairline, left-aligned), so Rename and Delete are no longer side by side. The "only channel" note moves with it.

## 8. Onboarding (`pages/Onboarding.tsx`)

- Step 1 (admins): "Create an ingest key for the Default channel." with the primary "Create a key" button and the once-only key. Under it, `p.muted`: "Or use the key printed in the server log at first boot:" and the log `CodeBlock`. Reviewers: "Ask an admin for an ingest key."
- Step 2: "Send a test message:" with the curl block (key filled once created), as today.
- Remove the "Web SDK" section entirely.
- Replace the closing sentence with a live indicator: `<p role="status" class="waiting"><span class="waiting-dot" aria-hidden="true"></span>Waiting for your first feedback…</p>`. The dot is the dusk "new" dot (8 px circle, `--dusk`). It pulses opacity 1 → 0.35 over 1.6 s only inside `@media (prefers-reduced-motion: no-preference)`; otherwise static. Polling stays at 5 s while visible (unchanged).

## 9. Inbox zero and filtered-empty stay plain

No new copy beyond §3. Baxter appears only where he does today.

## 10. Contrast

Measure with the WCAG formula (put the numbers in a CSS comment next to each change).
- **Selection tint:** add token `--selection: #D9CFD8` ("dusk tint one step deeper: 1.34:1 on paper; ink soft 5.1:1, dusk deep 4.4:1 on it") and use it for `.row-selected` instead of `--dusk-tint`. Between two selected rows use `border-top: 1px solid var(--dusk)` (2.6:1 on the tint) instead of kraft. Check every text on a selected row stays ≥ 4.5:1 (meta already switches to ink soft; anything still ink muted on a selected row moves to ink soft) and markers ≥ 3:1.
- **Input borders:** `.control` already uses `--border-control` (bark, 5.3:1). Audit every `input`, `select` and `textarea` (search box, tag editor input, rename field, token name, login) and make any that does not use `.control` or `--border-control` use it. Kraft stays for panels and dividers (not the only boundary of a control).
- **Ignored chip border, New dot and tag pills on selected rows:** verify ≥ 3:1 against `--selection` and fix with existing tokens if not.

## 11. Tests (by name; all must exist and pass)

`shell/NewCount.test.tsx`
- `shows the New count next to Inbox in the rail`
- `puts the New count in the document title`
- `leaves the title alone when nothing is new`
- `polls every 30 seconds only while visible`

`pages/Inbox.test.tsx` (add)
- `shows the new pill when the total grows`
- `Show reloads the list and hides the pill`
- `no pill on the Reviewed tab`
- `own deletions do not count as new`
- `inbox zero offers reviewed items when some wait`
- `filtered empty names the filters` (exists or add)

`feedback/FeedbackList.test.tsx` (add)
- `says Showing 50 of 312 when more pages exist`
- `auto-loads the next page when the end scrolls into view` (stub `IntersectionObserver`)
- `keeps the Load more button as a fallback`
- `relative times refresh after a minute` (if not already covered)

`lib/download.test.ts`
- `uses the filename from Content-Disposition`
- `prefers filename* when present`
- `throws on a failed response`

`feedback/ExportMenu` (in `Toolbar.test.tsx` or a new `ExportMenu.test.tsx`)
- `export failure shows Export failed. Try again.`
- `a successful export closes the panel`

`pages/System.test.tsx`
- `settings redirects to system`
- `shows the version in plain words in the footer`
- `could not be stored shows an alert icon and a line when above zero`

`lib/versionText.test.ts`
- `release build`, `dirty build is a development build`, `dev build`, `API process mode`

`shell/TopBar.test.tsx` / a Rail test
- `rail shows a Profile link`
- `nav says System, not Settings`

`admin/McpConfig.test.tsx`
- `fills the origin and token into both config forms`
- `disappears when the token is dismissed` (in a Profile test if simpler)

`admin/PeopleTable.test.tsx` (add)
- `the last admin's role is plain text with a note`
- `a role change asks for confirmation before saving`
- `cancelling a role change keeps the old role`
- `a temporary password shows Pending first sign-in`

`admin/ChannelPanel.test.tsx` (add) and Channels
- `New channel reveals the name field`
- `shows the item count and last received time`
- `shows No feedback yet for an empty channel`
- `key table shows last used or Never`
- `Delete channel is separate from Rename`

`pages/Onboarding.test.tsx` (add)
- `Create a key is step one`
- `no Web SDK line`
- `shows the waiting indicator as a status`

## 12. Done means

`cd web && npm ci && npm run typecheck && npm test && npm run build` green. Report: files, test count before and after, tests changed and why, deviations and why, and the measured contrast numbers.
