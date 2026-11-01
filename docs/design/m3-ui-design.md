# M3 design: admin UI

Author: Opus director · Milestone M3 · Sources: SPEC "Admin UI and design direction" and "Styleguide" (binding), API.md, ARCHITECTURE §9 and §13.
Single source for the M3 coders. Where this doc is silent, follow the Styleguide; where the Styleguide is silent, ask.

## 0. Ground rules (review fails on any of these)

- Colours only through the tokens in `web/src/styles/tokens.css`. No raw hex, `rgb()` or named colours anywhere else in `web/src` (artwork tokens included).
- Fraunces only for the wordmark and the page `<h1>`. Inter for everything else. Code and secrets use the system monospace stack (`--font-mono`), the one addition to the Styleguide (decision: a key must be legible character by character).
- At most one primary button per screen (dialogs are their own surface). Brick only on destructive actions: delete feedback, delete all for a user, delete channel, revoke key, remove person.
- Feedback text, metadata values and tag names are rendered as React text children only. No `dangerouslySetInnerHTML`, no Markdown. Full messages use `white-space: pre-line`.
- No `any`, TS strict. No external requests at runtime: fonts, icons and Baxter ship in the bundle; CSP enforces it (§8).
- No drop shadows, no gradients, no torn edges. The only texture is the grain on `body`.
- Copy is plain, factual, sentence case. Baxter never speaks. No puns, no exclamation marks.
- No Baxter animation in v1 (decision: the ear tilt is optional and adds motion for no function). The few colour/opacity transitions (≤ 150 ms) are switched off under `prefers-reduced-motion: reduce`.

## 1. Tokens (`tokens.css`, `:root`)

```css
/* palette: every Styleguide entry, by name */
--paper:#F4F1EA; --card-stock:#FAF8F4; --oat:#ECE4D6; --hairline:#DDD6CA; --kraft:#C2B39C;
--bark:#6F6151; --ink:#36302C; --ink-soft:#5A514A; --ink-muted:#73695F; --dusk:#8C7A8C;
--dusk-deep:#66586A; --dusk-tint:#E9E3E8; --dusk-ink:#4E4352; --sage-tint:#E2E8DC; --sage-ink:#46573D;
--brick:#9E5A4C;
/* artwork only (Baxter's snout and ear, from assets/brand) */
--baxter-snout:#FBF8F2; --baxter-ear:#8E7D69;
/* derived: dialog scrim (ink at 32 %) */
--scrim: rgb(54 48 44 / 0.32);
/* type */
--font-serif:"Fraunces",Georgia,serif; --font-sans:"Inter",system-ui,sans-serif;
--font-mono:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;
--fs-wordmark:24px; --fs-title:28px; --fs-message:16.5px; --lh-message:1.65;
--fs-preview:15px; --fs-section:15px; --fs-ui:14px; --fs-ui-sm:13px; --fs-meta:12.5px; --fs-label:12px;
--fw-regular:400; --fw-medium:500; --lh-ui:1.45;
/* space, 4 px scale */
--s-1:4px; --s-2:8px; --s-3:12px; --s-4:16px; --s-5:20px; --s-6:24px; --s-8:32px; --s-10:40px; --s-12:48px; --s-16:64px;
/* shape */
--radius-button:4px; --radius-panel:6px; --radius-pill:999px;
--border-hairline:1px solid var(--hairline); --border-kraft:1px solid var(--kraft);
--border-control:1px solid var(--bark);  /* inputs, selects, checkboxes: kraft is 1.8:1, bark 5.3:1 (WCAG 1.4.11) */
--focus-ring:2px solid var(--dusk-deep); --focus-offset:2px;
--target:44px; --rail-width:232px; --content-max:760px; --measure:33em; /* ≈ 62 characters of Inter */
--grain: url("data:image/svg+xml,…");     /* see below */
```

Grain: a 160 × 160 SVG with `<filter><feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" stitchTiles="stitch"/><feColorMatrix values="0 0 0 0 0.21  0 0 0 0 0.19  0 0 0 0 0.17  0 0 0 0.6 0"/></filter><rect width="100%" height="100%" filter="url(#n)" opacity="0.05"/>`, URL-encoded into `--grain`. `body { background: var(--grain), var(--paper); }`. Body only; panels, dialogs and the rail sit on flat colour.

Global (`base.css`): `body` Inter 14px/1.45 ink; `:focus-visible { outline: var(--focus-ring); outline-offset: var(--focus-offset); }` on everything, never `outline: none` without this replacement; links dusk deep, underline on hover; `::selection` dusk tint. `@media (prefers-reduced-motion: reduce) { *, *::before, *::after { transition: none !important; animation: none !important; scroll-behavior: auto !important; } }`.

Contrast (measured): ink muted 4.76 on paper and 5.06 on card stock, but **4.25 on oat and 4.24 on dusk tint**: never put ink muted text on oat or dusk tint; use ink soft there (6.1). Brick on paper 4.63, card stock on brick 4.92, paper on dusk deep 5.87, dusk dot 3.53 on paper and 3.15 on dusk tint.

## 2. Layout

```
┌──────────────┬───────────────────────────────────────────────────────┐
│ [b] FeedBaxter│  Page title (Fraunces 28)                             │
│               │  page content, max-width 760 px, padding 40 px 48 px  │
│ ▌Inbox        │                                                       │
│  Channels and │                                                       │
│  keys         │                                                       │
│  People       │                                                       │
│  Settings     │                                                       │
│               │                                                       │
│ admin@local…  │                                                       │
│ Admin         │                                                       │
│ Sign out      │                                                       │
└──────────────┴───────────────────────────────────────────────────────┘
```

- Skip link "Skip to content" is the first focusable element, visually hidden until focused, targets `<main id="main" tabindex="-1">`.
- Rail: `<nav aria-label="Main">`, width `--rail-width`, full height, sticky, paper background, hairline right border, padding 24 px 16 px. Top: link to `/` with the small Baxter mark at 28 px (`aria-hidden="true"`) and the wordmark "FeedBaxter" (Fraunces 24, ink). Nav items: 44 px tall, 14 px Inter, ink soft, radius 4; hover oat background, ink text; current item: dusk tint background, ink text, weight 500, a 3 px dusk deep inset bar on the left (`box-shadow: inset 3px 0 0 var(--dusk-deep)` is a bar, not a shadow) and `aria-current="page"`. Reviewers see only Inbox. Bottom: the user's email (link to `/profile`, ellipsis), role in ink muted ("Admin" / "Reviewer"), "Sign out" text action.
- Content: `<main>` with `<h1>` (Fraunces 28, 400, ink, margin-bottom 24) as its first child. On every route change: `document.title = "<Page> · FeedBaxter"` and focus moves to the `<h1>` (`tabindex="-1"`).
- Below 880 px the rail becomes a top bar: wordmark left, nav items in a row (wrap allowed, 44 px tall), profile link and "Sign out" at the end; content padding 24 px 16 px. At 768 px nothing overflows horizontally; below that, text wraps and tables scroll inside their own container (`overflow-x: auto`, `tabindex="0"`, labelled).
- Login and forced password change have no rail: a centered card stock panel, 400 px max, radius 6, kraft border, padding 32.

## 3. Components (all in `web/src/components/`, states listed for each)

Focus for every interactive element is the global ring. Disabled controls use `disabled` (or `aria-disabled="true"` when they must stay focusable to expose an explanation) and are paired with visible text saying why.

- **Button (primary)** `<Button variant="primary">`: height 44, padding 0 20, dusk deep fill, paper text, Inter 14/500, radius 4, no border. Hover: `--dusk-ink` fill. Active: `--ink` fill. Disabled: oat fill, ink soft text, cursor default. Loading: `aria-busy="true"`, disabled, label becomes the progress label passed by the caller ("Signing in…"), width locked so it does not jump.
- **Text action (secondary)** `<Button variant="text">`: no fill, ink soft, 14 px, underline 1 px kraft with 3 px offset (`text-decoration-color: var(--kraft)`), min height 44, padding 0 4. Hover: ink text, ink soft underline. Active: ink. Disabled: ink muted, no underline. Loading: progress label, `aria-busy`.
- **Destructive action** `<Button variant="danger">`: same as text action in brick text and brick underline at 50 % (`--brick` with `text-decoration-thickness:1px`). It always opens a confirm dialog. Inside the dialog the confirm is `<Button variant="danger-solid">`: brick fill, card stock text, otherwise like primary; hover/active darken to `--ink`.
- **Text input** `<TextField label hint error>`: visible `<label>` above (Inter 13/500 ink), input height 44, padding 0 12, card stock, `--border-control`, radius 4, 14 px ink, placeholder ink muted. Hover: border ink. Focus: ring (border stays). Disabled: oat background, ink soft text. Error: border brick, message below in brick 12.5 px with an alert icon, `aria-invalid="true"`, `aria-describedby` → hint and error ids. Hint in ink muted 12.5 px.
- **Select** `<SelectField label>`: native `<select>` styled like the input with `appearance:none`, a chevron-down icon absolutely positioned (pointer-events none), padding-right 36. Same states.
- **Checkbox** `<Checkbox label>`: native input, `appearance:none`, 18 px box, `--border-control` 1.5 px, card stock, radius 3; checked: dusk deep fill, paper check drawn with a CSS mask of the check icon; indeterminate: dusk deep fill with a paper dash. The `<label>` wrapper gives a 44 × 44 hit area. Label may be visually hidden but must exist.
- **Status marker** `<StatusMarker status>` 16 px, `aria-hidden` (the word is always next to it): new = solid dusk circle r 4.5 (the nose); reviewed = eye, ink soft; actioned = check-circle, sage ink; ignored = minus-circle, ink muted.
- **Status chip** `<StatusChip status>`: inline-flex, height 24, padding 0 8, radius 4, 12.5 px/500, marker + word ("New", "Reviewed", "Actioned", "Ignored"). New: dusk tint fill, dusk ink text. Reviewed: oat fill, ink soft text. Actioned: sage tint fill, sage ink text. Ignored: no fill, 1 px dashed kraft border, ink muted text (only on paper or card stock).
- **Tag pill** `<TagPill name onRemove?>`: oat fill, ink soft, 12.5 px, radius pill, height 24, padding 0 10. With `onRemove`: height 28 and a 28 px "×" button (`aria-label="Remove tag <name>"`) whose `::before` extends the hit area to 44 px.
- **Inbox row**: see §4 Inbox.
- **Filter bar**: one row (wraps): status segmented control, tag select, channel select (only with ≥ 2 channels), search input (`type="search"`, visible label "Search", no placeholder). Every filter has a visible label above it (13 px ink). Gap 12.
- **Segmented control** `<Segmented name label options>`: a `role="radiogroup"` `<fieldset>` with a visually hidden `<legend>`, native radios visually hidden, labels 44 px tall, padding 0 12, ink soft 14 px; the group has a kraft bottom rule. Checked: ink text, weight 500, 2 px dusk deep bottom bar. Hover: ink. Focus: ring on the label (`label:has(input:focus-visible)`). Arrow keys move as native radios do. Labels carry counts: "New 12".
- **Bulk action bar**: rendered above the list only when ≥ 1 row is selected; sticky top 0, card stock, hairline border, radius 6, padding 4 12, `role="toolbar"` `aria-label="Bulk actions"`. Content: "3 selected" (ink, 500) · text actions "Mark reviewed", "Mark actioned", "Ignore", "Mark as new" · tag input (label "Tag", 160 px) + text action "Add tag" · right-aligned text action "Clear selection". While a request runs, all actions are disabled and the count reads "Updating 3…".
- **Load more**: centered text action "Load more" under the list while `nextCursor` is non-null; loading label "Loading…". Under it, ink muted "Showing 50" (count loaded).
- **Toast** `<ToastProvider>`, `useToast()`: stack at bottom left of the content column (24 px from edges), max 3. Each: ink fill, paper text, 14 px, radius 6, padding 12 16, max-width 420, no shadow, a "Dismiss" icon button (x, `aria-label="Dismiss"`, 44 px hit area). Success/info auto-dismiss after 5 s, errors after 8 s; timers pause on hover and focus. Two always-present live regions: `role="status" aria-live="polite"` for info and `role="alert"` for errors; the visual toasts render inside them. Fade 150 ms (none under reduced motion).
- **Announcer** `useAnnounce()`: a separate visually hidden polite live region for keyboard results that do not need a toast (row focus is not announced; status changes are).
- **Confirm dialog** `<ConfirmDialog title body confirmLabel busyLabel onConfirm danger>` built on `<Dialog>`: portal to `document.body`, scrim `--scrim`, panel card stock, kraft border, radius 6, width min(440 px, 100vw − 32), padding 24, `role="dialog"` (or `alertdialog` for destructive), `aria-modal="true"`, `aria-labelledby` → title (Inter 16/500, not Fraunces), `aria-describedby` → body. The app root gets `inert` while open. Focus goes to Cancel for destructive dialogs, else to the first field; Tab and Shift+Tab cycle inside; Escape and Cancel close (not while busy); focus returns to the opener. Buttons right-aligned: "Cancel" text action, then the confirm. Errors from the action render inside the dialog in brick with `role="alert"` and the dialog stays open.
- **Empty state** `<EmptyState mark heading body action?>`: centered, padding 64 0, Baxter full mark at 128 px (`role="img"` `aria-label="FeedBaxter"`), heading Inter 16/500 ink, body ink soft 14 px, max-width 36em, optional text action. Used for: onboarding (inside its own layout), inbox zero, no matches after filters, monthly limit reached.
- **Code block** `<CodeBlock code label>`: `<figure>` with a caption row (label in 12.5 px ink soft, "Copy" text action on the right) and `<pre tabindex="0" aria-label={label}>` oat fill, ink text, 13 px mono, radius 6, padding 16, `overflow-x:auto`, `white-space:pre`. Copy uses `navigator.clipboard.writeText` when available, else a hidden textarea + `document.execCommand("copy")` (self-host over plain HTTP has no clipboard API). After copying the action reads "Copied" for 2 s and the announcer says "Copied to clipboard."; on failure: toast "Could not copy. Select the text and copy it manually."
- **Secret shown once** `<SecretOnce title value note onDone>`: card stock panel, kraft border, radius 6, padding 20, `role="region"` `aria-label={title}`, focused when it appears. Title Inter 15/500 (e.g. "Copy this key now"), the value in a read-only mono field (oat, 44 px, full width, selected on focus) with a "Copy" text action, the note in ink soft ("It is shown only once. Only a hash is stored."), and a "Done" text action that removes the panel.
- **Email warning** `<EmailWarning userId>`: renders only when `looksLikeEmail(userId)`; one line, alert icon in bark + ink soft 13 px: "This user ID looks like an email address. User IDs should be your internal, non-identifying IDs."
- **Inline alert** `<InlineAlert>` for page-level load errors: card stock, 1 px brick border-left 3 px, ink text, `role="alert"`, with a "Try again" text action.
- **Icons** `web/src/components/icons.tsx`: hand-written inline SVG, `viewBox="0 0 16 16"`, `width/height 16`, `fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"`, `aria-hidden="true" focusable="false"`. Set: dot (filled, `fill="currentColor"`), eye, check-circle, minus-circle, check, x, copy, search, arrow-left, chevron-down, alert-circle, download. Colour comes from CSS (`color: var(--…)`), default ink.
- **Baxter** `web/src/components/Baxter.tsx`: `<BaxterFull size>` and `<BaxterSmall size>` as inline SVG paths copied from `assets/brand/*.svg` with fills replaced by `var(--oat)`, `var(--baxter-snout)`, `var(--baxter-ear)`, `var(--ink)`, `var(--card-stock)` (catchlight), `var(--dusk)` (nose). Props `decorative` (→ `aria-hidden`) or label (→ `role="img" aria-label="FeedBaxter"`). Favicon: `web/public/favicon.svg` = copy of `baxter-small.svg` (an asset file, not source).

## 4. Screens

Routes: `/login`, `/change-password`, `/` (inbox or onboarding), `/feedback/:id`, `/user/:userId` (encoded with `encodeURIComponent`), `/channels`, `/people`, `/settings`, `/profile`, `*` (not found). Admin routes rendered for a reviewer show the not-found page.

**App start / auth gate.** `GET /api/me` on every full load. Loading: blank paper (no spinner) for the first 300 ms, then "Loading…" in ink muted centered. 401 → `/login?next=<path>`. `mustChangePassword` → `/change-password`. Any later API call: 401 → clear the user, go to `/login?next=…`, toast "Your session ended. Sign in again."; 403 `password_change_required` → `/change-password`.

**Login** — no rail. Panel: the small mark 28 px (decorative) + wordmark, then `<h1>` "Sign in", TextField "Email" (`type=email autocomplete=username`), TextField "Password" (`type=password autocomplete=current-password`), primary "Sign in" (loading "Signing in…"). Error above the fields, `role="alert"`, brick text: the server message ("Email or password is incorrect."). Footnote ink muted 12.5: "Forgot your password? Ask an admin to reset it. Admins can run `feedbaxter reset-admin` in the container." API: `POST /api/auth/login`. Success → `next` (only same-origin paths starting with `/`) or `/`; if `user.mustChangePassword` → `/change-password`.

**Change password (forced)** — no rail. `<h1>` "Choose a new password"; ink soft "You signed in with a temporary password. Choose a new one to continue."; fields "Temporary password" (current), "New password" (hint "10 to 256 characters."), "Confirm new password"; primary "Save password" ("Saving…"); text action "Sign out". Client checks: length → "Use 10 to 256 characters."; mismatch → "The new passwords do not match." API: `POST /api/auth/password`, then `GET /api/me`; toast "Password changed."; go to `next` or `/`.

**Inbox** `/` — first `GET /api/stats` (total and per-status counts) and `GET /api/channels`, `GET /api/tags`. If `stats.total === 0` render Onboarding instead (no filters). Otherwise:
```
Inbox                                                     Export CSV  Export JSON
[New 12][Reviewed 30][Actioned 10][Ignored 3][All 55]  Tag [All tags ▾]  Channel [All ▾]  Search [______]
┌ 3 selected · Mark reviewed · Mark actioned · Ignore · Mark as new · Tag [___] Add tag   Clear selection ┐
[☐] Select all on this page
───────────────────────────────────────────────────────────────────────────────────
[☐] ●  The export button does nothing on Safari and I have tried twice…        2 h ago
       New · cust-4821 · web · 2.4.1 · Default · (bug) (export)
───────────────────────────────────────────────────────────────────────────────────
                                 Load more
                                Showing 50
```
- Filters live in the URL query (`status`, `tag`, `channel`, `q`); default status is `new` (omitted from the URL). "All" sends no status. Search is debounced 300 ms, max 200 characters. Changing a filter resets the list and selection.
- Rows: `<ul aria-label="Feedback">` of `<li>`; hairline divider between rows, no boxes. Grid: checkbox (44) · marker (16, top-aligned with the preview) · body · time. Preview: first 200 characters of the message with line breaks collapsed to spaces, one line, ellipsis, 15 px ink, inside a link to `/feedback/:id` whose accessible name is the preview. Metadata line below (12.5 px ink muted, separated by " · "): status word, userId (or "No user ID"), platform, app version, channel name (only with ≥ 2 channels; "Deleted channel" when null), then up to 3 tag pills and "+N". Time on the right: `<time dateTime title={absolute}>` relative, 12.5 px ink muted. Hover: oat background is not allowed (ink muted on oat fails); hover shows the preview underlined. Selected row: dusk tint background and the metadata switches to ink soft. Current (keyboard) row: the focus ring on the row link plus a 3 px dusk deep left bar.
- Roving focus: exactly one row is current; its link and checkbox have `tabindex=0`, other rows' `-1`. Tab order: filters → select all → current row link → its checkbox → Load more.
- Keyboard (inactive while focus is in an input, select or textarea, while a dialog is open, or with Ctrl/Meta/Alt): `j`/`ArrowDown` next row, `k`/`ArrowUp` previous (at the last loaded row with a next cursor, `j` loads more); `x` toggle selection of the current row; `Enter` open; `e` reviewed, `a` actioned, `i` ignored, `n` mark as new — applied to the selection if any, else to the current row; `Escape` clear selection; `/` focus search; `?` toggle the shortcuts panel. In the search field, `Escape` blurs it back to the current row.
- Status change: `POST /api/feedback/status` (selection) or `PATCH /api/feedback/{id}` (single). Rows that no longer match the status filter leave the list; focus moves to the next row (or previous, or the empty state's heading). Announce via the announcer: "Marked 3 as reviewed." (singular "Marked 1 as reviewed."); with `notFound`: append " 1 was no longer there." Then refresh stats counts. Errors → error toast with the server message.
- Bulk add tag: `POST /api/feedback/add-tags` with the one tag; announce "Added tag bug to 3."; refresh tags.
- Select all on this page: header checkbox, indeterminate when some are selected. Selection survives Load more, is cleared by filter changes.
- Shortcuts panel: text action "Keyboard shortcuts" under the list toggles a `<dl>` (`aria-expanded`).
- Export: two text actions with download icon, `<a href="/api/feedback/export?format=csv&<current filters>" download>`; same for JSON.
- Back from detail restores the list from an in-memory cache keyed by the query string and returns focus to the row that was opened.
- Loading first page: "Loading feedback…" ink muted. Error: InlineAlert "Could not load feedback. <message>" + "Try again".
- Empty (status new, no tag/channel/q): EmptyState heading "No new feedback", body "New feedback appears here as it arrives." Empty with any other filter: heading "Nothing matches these filters", body "Try another status or tag, or clear the search.", action "Clear filters" (back to `/`).

**Onboarding** (inside the shell, at `/` while `stats.total === 0`). `<h1>` "Send your first feedback". BaxterFull 128 decorative at the top right of the intro (stacked under 880 px). Body:
1. "Your Default channel has an ingest key. It was printed once in the server log at first boot:" + CodeBlock `docker compose logs feedbaxter | grep "ingest key"` (label "Find the key in the log").
2. Admin: "Lost it? Create a new key for the Default channel." + primary "Create a key" (`POST /api/channels/{defaultId}/keys`; default channel = the one named "Default", else the oldest) → SecretOnce "Copy this key now". Reviewer: "Ask an admin for the ingest key." (no button).
3. "Then send a test message:" + CodeBlock (label "First API call") built from `location.origin`:
   `curl -X POST <origin>/v1/feedback \` / `  -H "Authorization: Bearer <key or fbx_your_key>" \` / `  -H "Content-Type: application/json" \` / `  -d '{"message": "The export button does nothing on Safari", "userId": "usr_8f2a91"}'`
4. Section "Web SDK": ink soft "The Web SDK is not released yet. Until it is, call the API as shown above from your app." (placeholder, decision: no invented package name).
5. Ink muted: "This page switches to the inbox when the first feedback arrives." Poll `GET /api/stats` every 5 s while visible (pause when `document.hidden`); on `total > 0` show the inbox and toast "First feedback received.".

**Feedback detail** `/feedback/:id` — `GET /api/feedback/{id}`, `GET /api/tags` (suggestions), `GET /api/channels` (count only).
```
← Inbox
Feedback                                                (h1)
[● New] · cust-4821 · ios · 2.3.0 · Default · 1 Mar 2026, 09:30 (2 h ago)
⚠ This user ID looks like an email address…              (only if it does)

The export button does nothing on Safari.                (16.5/1.65 ink, pre-line, max 33em)
1. Open reports
2. Press export

[Mark reviewed]  Mark actioned · Ignore                  (primary = next step)
Tags   (bug ×) (export ×)  Tag [______] Add tag
Metadata   screen  export / plan  pro                     (<dl>, values as text; objects JSON.stringify'd)
History    Received · 1 Mar 2026, 09:30
           New → Reviewed · a@example.com · via UI · 2 Mar 2026, 10:00
More from this user →  /user/cust-4821
Delete feedback                                          (admin, brick)
```
- "← Inbox" text action goes back in history when the previous entry is the inbox, else to `/`. Metadata line uses the chip for status; userId links to the per-user view; null fields are omitted ("No user ID" when userId is null).
- Primary button: new → "Mark reviewed"; reviewed → "Mark actioned"; actioned/ignored → none. Text actions for every other status ("Mark reviewed", "Mark actioned", "Ignore", "Mark as new"), excluding the current one. `e`/`a`/`i`/`n` work here too. API `PATCH /api/feedback/{id}`; re-fetch the detail for history; announce "Marked as reviewed.".
- Tags: add via `POST /api/feedback/add-tags` `{ids:[id],tags:[name]}` (Enter or "Add tag"; `<datalist>` of existing tags), remove via `remove-tags`. Announce "Added tag bug." / "Removed tag bug.". Validation: 1–64 characters → "Tags are 1 to 64 characters.".
- Metadata null → "No metadata." History `via` shown as "via UI" / "via MCP"; `userEmail` null → "a removed user".
- Delete (admin): ConfirmDialog title "Delete this feedback?", body "This permanently deletes the feedback. It still counts toward this month's submission limit.", confirm "Delete feedback", busy "Deleting…". `DELETE /api/feedback/{id}` → navigate to the inbox, toast "Feedback deleted.".
- 404: `<h1>` "Feedback not found", "It may have been deleted.", text action "Back to the inbox".

**Per-user view** `/user/:userId` — `GET /api/feedback?userId=…` (cursor pagination, all statuses). `<h1>` "Feedback from <userId>" (the id as text). EmailWarning. Same row list and keyboard as the inbox (no filter bar; status chips not needed, markers + status word as in rows), Load more, export links with `userId`. Admin: "Delete all feedback for this user" (brick) → ConfirmDialog title "Delete all feedback from <userId>?", body "This permanently deletes every feedback item from this user. Deleted feedback still counts toward this month's submission limit.", confirm "Delete all feedback", busy "Deleting…". `POST /api/feedback/delete-by-user` → toast "Deleted 12 feedback items." (1 → "Deleted 1 feedback item."), list empties. Empty: plain ink soft "No feedback from this user." (no Baxter).

**Channels and keys** `/channels` (admin) — `GET /api/channels`, `GET /api/keys`.
```
Channels and keys
New channel name [__________]  [Create channel]                       (primary)
┌ Default · created 1 Mar 2026                      Rename   Delete channel ┐
│ Keys                                                        Create key   │
│ Key            Created        Status                                     │
│ fbx_ab12cd34…  1 Mar 2026     Active                          Revoke     │
│ fbx_99xx11aa…  2 Feb 2026     Revoked 3 Feb 2026                         │
└───────────────────────────────────────────────────────────────────────────┘
```
- Create: `POST /api/channels`; errors inline under the field (409 "A channel with that name already exists."). Success toast "Channel Web created.".
- Each channel is a `<section aria-labelledby>` panel (card stock, hairline border, radius 6, padding 20, gap 16 between panels). Name Inter 15/500.
- Rename: inline TextField (label "Channel name") + text actions "Save", "Cancel"; Escape cancels; focus returns to "Rename". `PATCH /api/channels/{id}`; toast "Channel renamed.".
- Delete (brick): ConfirmDialog "Delete channel <name>?", body "Its N active keys are revoked immediately, so apps using them will start failing. Feedback already received is kept." (N = 0 → "It has no active keys. Feedback already received is kept."), confirm "Delete channel". With one channel the action is replaced by ink muted text "The only channel cannot be deleted.".
- Keys: `<table>` with `<caption class="visually-hidden">Keys for <name></caption>` and `<th scope="col">` Key, Created, Status, (action, `<th>` text visually hidden "Actions"). Prefix in mono followed by "…". Revoked rows in ink muted with "Revoked <date>"; active rows "Active". "Create key" text action → `POST /api/channels/{id}/keys` → SecretOnce in that panel, title "Copy this key now", note "It is shown only once. Only a hash is stored." "Revoke" (brick) → ConfirmDialog "Revoke key <prefix>…?", body "Apps using this key will start failing immediately. This cannot be undone.", confirm "Revoke key"; `POST /api/keys/{id}/revoke`; toast "Key revoked.".

**People** `/people` (admin) — `GET /api/users`, `GET /api/me`.
- Add form: TextField "Email" + SelectField "Role" (Reviewer default, Admin) + primary "Add person" ("Adding…"). `POST /api/users` → SecretOnce title "Temporary password for <email>", note "Share it with them directly. They must change it when they first sign in. It is shown only once.". Errors inline (400/409 messages).
- Table: Email (+ " (you)" for self; ink muted "Temporary password" when `mustChangePassword`), Role (SelectField with visually hidden label "Role for <email>"; change → `PATCH /api/users/{id}`, toast "<email> is now an admin." / "…a reviewer."), Added (date), Actions: "Reset password" text action → ConfirmDialog (not destructive styling) "Reset the password for <email>?", body "Their sessions end and they get a temporary password to change at next sign-in.", confirm "Reset password" (primary style) → SecretOnce; "Remove" (brick) → ConfirmDialog "Remove <email>?", body "They are signed out and their personal access tokens stop working.", confirm "Remove person"; toast "<email> removed.".
- Self row: no Reset (help text "Change your own password in your profile.") and no Remove. Last admin (only one user with role admin): their role select disabled and Remove absent, with ink muted text "The last admin cannot be removed or made a reviewer." Server 409s still surface as error toasts.

**Settings → Operations** `/settings` (admin) — `<h1>` "Settings", `<h2>` "Operations" (Inter 15/500). `GET /api/ops` on mount and every 10 s while visible (pause when hidden).
- A `<dl>` grid of panels (2 columns ≥ 880 px, 1 below; card stock, hairline, radius 6, padding 16): "Waiting in queue" `n`; "Oldest waiting" (`0` → "None", else "2 min 5 s"); "Could not be stored" (DLQ) `n`; "Accepted today" `n`; "Accepted this month" `n`; "Database size" ("1.0 MB", binary units, one decimal); "Version" "0.2.0 · all mode · SQLite" (Postgres). `<dt>` 12.5 ink muted, `<dd>` 20 px Inter ink, tabular numbers.
- Notes under the grid, ink soft: DLQ > 0 → "Some submissions could not be stored. See "Signals" in OPS.md." Oldest > 60 s → "Processing is behind. Submissions are safe in the queue."
- Footer ink muted: "Updated 10:31:05. Refreshes every 10 seconds." + text action "Refresh now".
- Unavailable (any error or network failure): replace the values with "Operations data is unavailable right now. Retrying every 10 seconds." (InlineAlert without "Try again"; keep "Refresh now"). A 500 from NATS lands here.

**Profile** `/profile` — `GET /api/me`, `GET /api/tokens`.
- `<h1>` "Profile". Section "Account": email, role.
- Section "Change password": fields "Current password", "New password" (hint), "Confirm new password"; primary "Change password". Toast "Password changed. Other sessions were signed out."
- Section "Personal access tokens": ink soft "Tokens let your own AI client use the MCP server with your role. Keep them secret and never put them in an app." TextField "Token name" + text action "Create token" → SecretOnce "Copy this token now". Table: Name, Prefix (mono + "…"), Created, Last used ("Never"), action "Revoke" (brick) → ConfirmDialog "Revoke token <name>?", "Anything using this token stops working immediately.", confirm "Revoke token". Empty: "No tokens yet.".

**Not found** `*` — `<h1>` "Page not found", text action "Go to the inbox".

## 5. Accessibility checklist (implemented, then verified by Fable)

Visible focus everywhere (global ring, never removed). 44 px targets for every control (pill × via extended hit area). Every control has a programmatic label; icon-only buttons have `aria-label`. Status always marker + word. Contrast per §1, including hover, selected and disabled text. Live regions: toasts (polite/assertive) and the announcer (bulk results, status changes, copy). Skip link. `aria-current="page"` on the nav item. Lists of feedback are `<ul>`; keys, people and tokens are `<table>` with `<th scope="col">` and a caption. Dialogs: `role="dialog"`/`alertdialog`, `aria-modal`, labelled, focus trapped, Escape, focus returned, background `inert`. Forms: errors linked by `aria-describedby`, `aria-invalid`, first invalid field focused on submit. Headings in order (one h1 per page, h2 for sections). `lang="en"` on `<html>`. Reduced motion per §0. Zoom to 200 % without loss at 1280 px.

## 6. Helpers (pure, unit-tested)

`lib/time.ts`: `relativeTime(iso, now)` → "just now" (< 45 s), "N min ago" (< 60 min), "N h ago" (< 24 h), "yesterday", "N days ago" (< 7 d), then "1 Mar", or "1 Mar 2025" for another year; `absoluteTime(iso)` → "1 Mar 2026, 09:30" (en-GB, local time). `lib/format.ts`: `bytes(n)`, `duration(seconds)`, `plural(n, one, many)`. `lib/email.ts`: `looksLikeEmail(s)` = trimmed matches `^[^\s@]+@[^\s@]+\.[^\s@]+$`. `lib/filters.ts`: query ⇄ `Filters` and `toApiParams(filters)` (status default new, "all" omitted). `lib/cursorList.ts`: reducer for `{items, nextCursor, loading, error}` with reset, page loaded (dedupe by id, ignore stale request ids), remove ids, replace item. `lib/shortcuts.ts`: `shortcutFor(KeyboardEvent-like, context)` → action or null.

## 7. API client (`lib/api.ts`)

`request<T>(method, path, body?)`: `fetch(path, {method, credentials:"same-origin", headers:{"X-Requested-With":"fetch", Accept:"application/json", +"Content-Type":"application/json" when body}})`. 204 → `undefined`. Non-2xx → `ApiError {status, code, message}` from the body; a body that is not the error shape → code `internal`, message "Something went wrong. Try again in a moment.". `fetch` rejection → status 0, code `network`, "Could not reach the server. Check your connection and try again.". 401 and 403 `password_change_required` also notify a subscriber (`onAuthProblem`) that the auth gate uses. Typed wrappers per endpoint in `lib/endpoints.ts` with the API.md shapes in `lib/types.ts`. No fetch wrapper library, no state library.

## 8. Serving (Go)

`web/embed.go` (`package web`, `//go:embed all:dist`, `var Dist embed.FS`); `web/dist/.gitkeep` committed (the build script recreates it). `internal/ui.Handler(fsys fs.FS) http.Handler`, mounted at `/` in `api` and `all` modes. GET/HEAD only (else 405). `/assets/*` existing file → `Cache-Control: public, max-age=31536000, immutable`; missing → 404. Other existing non-dot files at the dist root (`favicon.svg`, font licences) → `Cache-Control: no-cache`. Paths under `/v1/`, `/mcp`, `/api` → JSON 404 `not_found` (never the SPA). Everything else → `index.html` with `Cache-Control: no-store`. `.woff2` served as `font/woff2`. Headers on every UI response: `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, `X-Frame-Options: DENY`, `Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'`. With no `index.html` in the embed: 503 text "The admin UI is not built into this binary. Run make ui, then rebuild."

## 9. Decisions made during review

- List shortcuts also work when focus is on the page itself (`body`, `main` or the `<h1>`, e.g. right after load); the first `j`/`k` from there lands on the current row. Found in the browser: otherwise `j` did nothing until the user tabbed into the list.
- Elements that only receive focus from script and are not controls (`h1`, `h2`, `main` with `tabindex="-1"`, the secret-shown-once region) draw no focus ring (`outline-color: transparent`). Every interactive element keeps the ring.
- The inbox list cache is cleared whenever the signed-in user changes or signs out.
- "Monthly limit reached" is not built: no API field tells the UI a limit is hit (`/api/ops` has counters, not limits). Open issue for the architect.
- The Inbox nav item is `aria-current="page"` on `/`, `/feedback/*` and `/user/*`.
