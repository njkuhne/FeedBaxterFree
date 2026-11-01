# M3 brief C: channels and keys, people, settings → operations

Author: Opus director · Milestone M3 · Design source: `docs/design/m3-ui-design.md` §3 and §4 (Channels and keys, People, Settings → Operations). Copy there is binding word for word.

Brief A is done: build on `web/src/components/*` (Button, TextField, SelectField, ConfirmDialog, SecretOnce, InlineAlert, PageTitle, VisuallyHidden, Toast, Announcer), `web/src/lib/*` (endpoints, types, time, format), `web/src/shell/*`. Reuse; do not re-implement. Brief B runs in parallel and owns the inbox, detail, per-user and onboarding pages (`pages/Inbox.tsx`, `Onboarding.tsx`, `FeedbackDetail.tsx`, `PerUser.tsx`, `web/src/feedback/*`); do not touch those. Brief D runs in parallel on Dockerfile/CI/docs.

## Files

```
web/src/pages/Channels.tsx   web/src/admin/ChannelPanel.tsx   web/src/admin/KeyTable.tsx
web/src/pages/People.tsx     web/src/admin/PeopleTable.tsx
web/src/pages/Settings.tsx   web/src/admin/OpsGrid.tsx        web/src/admin/usePolling.ts
web/src/admin/*.test.tsx     web/src/admin/usePolling.test.ts
web/src/styles/admin.css     # imported once
```
All three routes are already wrapped in `AdminOnly` by brief A.

## Channels and keys (`/channels`)

- Load `listChannels()` and `listKeys()` in parallel; group keys by `channelId`. Loading "Loading channels…" ink muted; error → InlineAlert "Could not load channels. <message>" + "Try again".
- Create form at the top: TextField "New channel name" + primary "Create channel" (loading "Creating…"). Client check: trimmed 1–64 characters → "Use 1 to 64 characters.". Server errors inline under the field (`aria-invalid`, describedby). Success: clear the field, add the channel, toast "Channel <name> created.".
- One `<section aria-labelledby>` panel per channel (card stock, hairline border, radius 6, padding 20, 16 px gap). Header row: name (`<h2>` Inter 15/500), "created <date>" ink muted (date format "1 Mar 2026" — add `shortDate(iso)` to `lib/time.ts` with a test), actions "Rename" (text) and "Delete channel" (danger).
- Rename: inline TextField "Channel name" prefilled + text actions "Save" and "Cancel"; Enter saves, Escape cancels; focus returns to "Rename" afterwards. Toast "Channel renamed.". Errors inline.
- Delete: ConfirmDialog (danger) "Delete channel <name>?" with body by active key count N: N ≥ 1 → "Its N active keys are revoked immediately, so apps using them will start failing. Feedback already received is kept." (N = 1 → "Its 1 active key is revoked immediately, …"), N = 0 → "It has no active keys. Feedback already received is kept.". Confirm "Delete channel", busy "Deleting…". On success remove the panel, move focus to the page `<h1>`, toast "Channel <name> deleted.". With exactly one channel render ink muted text "The only channel cannot be deleted." instead of the action.
- Keys: section title "Keys" (Inter 13/500) + text action "Create key" (loading "Creating…") → `createKey` → SecretOnce in that panel: title "Copy this key now", note "It is shown only once. Only a hash is stored."; add the key to the table. Table per design: `<caption class="visually-hidden">Keys for <name></caption>`, `<th scope="col">` Key, Created, Status, and a visually hidden "Actions" header. Prefix mono + "…". Active: "Active"; revoked: row text ink muted, "Revoked <date>". "Revoke" (danger) only on active keys → ConfirmDialog "Revoke key <prefix>…?", body "Apps using this key will start failing immediately. This cannot be undone.", confirm "Revoke key", busy "Revoking…"; replace the key with the response; toast "Key revoked."; focus to the panel heading. No keys: ink soft "No keys yet.".

## People (`/people`)

- Load `listUsers()`; the current user comes from `useAuth()`.
- Add form: TextField "Email" (`type=email`), SelectField "Role" (options Reviewer — value `reviewer`, default — and Admin), primary "Add person" (loading "Adding…"). Errors inline under the email field. Success: SecretOnce title "Temporary password for <email>", note "Share it with them directly. They must change it when they first sign in. It is shown only once."; append the user; clear the form.
- Table (`<caption class="visually-hidden">People</caption>`): Email (+ " (you)"; ink muted "Temporary password" on a second line when `mustChangePassword`), Role (SelectField with visually hidden label "Role for <email>"; on change → `setRole`, toast "<email> is now an admin." or "<email> is now a reviewer."; on error revert the select and show an error toast), Added (`shortDate`), Actions.
- Actions: "Reset password" (text) → ConfirmDialog (not danger) "Reset the password for <email>?", body "Their sessions end and they get a temporary password to change at next sign-in.", confirm "Reset password", busy "Resetting…" → SecretOnce "Temporary password for <email>" (same note) shown above the table; mark the row as temporary. "Remove" (danger) → ConfirmDialog "Remove <email>?", body "They are signed out and their personal access tokens stop working.", confirm "Remove person", busy "Removing…" → remove the row, toast "<email> removed.".
- Self row: no Reset (ink muted "Change your own password in your profile." linking to `/profile`) and no Remove. Last admin (exactly one user with role admin): their role select is `disabled` and Remove absent, with ink muted text "The last admin cannot be removed or made a reviewer." (associate it with the select via `aria-describedby`). If the current user changes their own role to reviewer (allowed when another admin exists), call `me()` and update the auth user; the admin pages then become Not found, so navigate to `/` with toast "You are now a reviewer.".

## Settings → Operations (`/settings`)

- `<h1>` "Settings", `<h2>` "Operations". `usePolling(fn, 10_000)`: runs immediately, then every 10 s; pauses while `document.visibilityState === "hidden"` and runs once on becoming visible; `refresh()` runs now and resets the timer; never overlaps requests; cleans up on unmount.
- Grid `<dl>` per design §4 with the exact labels: "Waiting in queue", "Oldest waiting" ("None" for 0, else `duration(seconds)`), "Could not be stored", "Accepted today", "Accepted this month", "Database size" (`bytes()`), "Version" ("0.2.0 · all mode · SQLite" / "Postgres"). Numbers with `toLocaleString("en-GB")` and `font-variant-numeric: tabular-nums`. 2 columns ≥ 880 px, 1 below.
- Notes: DLQ > 0 → "Some submissions could not be stored. See "Signals" in OPS.md."; oldest > 60 s → "Processing is behind. Submissions are safe in the queue.".
- Footer: "Updated <HH:MM:SS>. Refreshes every 10 seconds." + text action "Refresh now".
- Any error (including 500 and network): replace the grid with InlineAlert (no "Try again") "Operations data is unavailable right now. Retrying every 10 seconds."; footer stays. Recovery restores the grid. Do not toast on poll errors.
- Updates are not announced (no live region on the grid) to avoid noise every 10 s.

## Styling

Only tokens. Tables use the `.table` classes from components.css; wrap each table in a `.table-scroll` div with `tabindex="0"` and `aria-label` (the caption text) so it scrolls by keyboard at narrow widths. Admin panels: card stock, `--border-hairline`, `--radius-panel`. At 768 px no horizontal page scroll.

## Tests

- `ChannelPanel.test.tsx`: delete dialog body text for N = 0, 1, 2; "The only channel cannot be deleted." with one channel; revoke shows only on active keys; created secret appears in SecretOnce once.
- `PeopleTable.test.tsx`: last admin's select disabled and Remove absent with the explanation; self row has no Remove/Reset; role change calls PATCH and toasts the sentence.
- `OpsGrid.test.tsx`: renders labels and formatted values; DLQ note; behind note; unavailable state on error.
- `usePolling.test.ts` (fake timers): runs at 0 and 10 s; pauses when hidden; no overlap.
- `time.test.ts`: `shortDate`.

## Acceptance

- `cd web && npm run typecheck && npm test && npm run build` pass; paste outputs.
- Against `make dev` (after `make ui`, or Vite 5173 + `make dev`) as the admin: create, rename and delete a channel; create and revoke keys (and confirm with curl that a revoked key gets 401 on `/v1/feedback`); add a reviewer, change roles, reset a password, remove a person; verify the last-admin explanation; open Settings and watch it refresh; stop nothing (the unavailable state is covered by the unit test). Sign in as the reviewer (forced change first) and confirm the rail shows only Inbox and `/channels` is Not found. Screenshots of each page; console clean; network same-origin only. Report what you checked.
- No git commands of any kind.
