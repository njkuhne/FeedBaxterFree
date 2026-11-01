# FeedBaxter UX review: architect's verdict

Seven Opus product designers with different backgrounds reviewed the same app against the same brief (simplicity; reading and reviewing feedback first). 894 suggestions merged to 283 distinct ones; 40 were raised by all seven. The merged report is REPORT.md; the seven originals are review-1.md to review-7.md.

## Do

In this order. Everything in phases 1 and 2 is front-end only except the one marked backend.

### Phase 1: make the triage loop safe and fast (the core of the brief)

1. **Visible toast with Undo for every status change**, single, bulk and detail, plus `z` to undo. Undo reverses the change and writes a second history entry; that is honest and needs no backend. (7 of 7)
2. **Detail page becomes the reading surface.** Previous/Next with a position line ("4 of 17 in New"), `j`/`k` on detail, Escape or `u` back to the list with the row focused, and auto-advance to the next item after a decision, on by default, with the Undo toast as the safety net. Back returns to where you came from (the per-user page bug). (7 of 7)
3. **Message first on detail.** Drop the visible "Feedback" title; keep a visually hidden h1. The spec's Fraunces-title rule gets one documented exception for this page. Keep one primary button; fix the status actions in a constant order and position with parallel labels: Mark reviewed, Mark actioned, Mark ignored, Mark new. (7 of 7)
4. **Rows that can be read without opening.** Two-line clamp with line breaks kept, whole row clickable, drop the status word on single-status tabs, drop the channel when only one exists or one is filtered, drop the user ID on the per-user page, platform display names (iOS, macOS, Web). (7 of 7)
5. **Honest tab counts.** Extend `/api/stats` to accept the list filters (tag, channel, q, userId). Backend, medium. Hiding the counts is the fallback, not the plan. (7 of 7)
6. **One compact header.** Tabs, then one toolbar line: search with placeholder and clear button, compact Tag and Channel menus with counts, one Export control that says what it will export, select-all as a bare checkbox in the list header. The bulk bar swaps into that header line at the same height (no layout shift), on one line, without the no-op action for the current tab, with tag suggestions, Remove tag, and "Select all N matching" up to the 500 cap. (7 of 7)
7. **Search that finds things.** With a query, search all statuses and show it; highlight the match; a pasted `usr_…` offers "Show feedback from usr_…". (7 of 7)
8. **Tags.** One shared tag editor (pills, "+ Add tag", suggestions with counts) used on detail and in the bulk bar; `t` opens it; 44 px remove targets; undo on remove. (7 of 7)

### Phase 2: mobile, as a supported read-and-triage surface (not bulk)

9. Compact top bar with wordmark and a menu button, one scrolling row of status tabs, Tag and Channel behind a Filters button, Export tucked away, sticky bottom bar on detail with the status actions and Next. (7 of 7)

### Phase 3: small things worth their cost

10. `?` opens a real shortcuts dialog; shortcut hints on buttons; `r` as alias for reviewed; range selection with Shift. (6 to 7 of 7)
11. Copy message and Copy link on detail; pretty-printed nested metadata; history written as sentences; per-user summary line; delete-all in its own titled section with a count in the confirmation. (7 of 7)
12. Relative times that update; keypresses buffered rather than dropped while a request is in flight; "N new · Show" pill when new items arrive; export failures shown in the UI; descriptive export filenames (backend, small). (5 to 7 of 7)
13. Contrast fixes: Ignored chip border, New dot on selected rows, tag pills on selected rows, input borders. (2 to 3 of 7, but measured, not opinion)
14. Admin tidy: rename Settings to System and move the version line to a footer in plain words; a visible Profile link and a ready-to-paste MCP config next to a new token; "Create channel" demoted; per-channel count and last received and key last-used (backend, small, both earn their place because nobody revokes a key without them); last admin's role as text; "could not be stored" with an alert icon, not brick. (7 of 7)
15. Onboarding: "Create a key" as step one, drop the "Web SDK not released" line, live waiting indicator. (5 to 6 of 7)

## Do not

- **No split-pane reading view.** Previous/Next plus auto-advance gives the same flow without a second layout to design, test and keep accessible. (4 reviewers refuse it; 2 asked.)
- **No swipe gestures on mobile.** Undiscoverable, accident-prone, and they fight the plain style. The sticky bottom bar does the job.
- **No notes, comments, assignment, mentions.** This is the line between a feedback inbox and a helpdesk. Six of seven refuse it.
- **No saved views.** The URL is the saved view; filters are already addressable.
- **No bulk delete.** Per-item and per-user deletion cover the real cases; bulk permanent deletion is an incident waiting to happen.
- **No sort menu, no multi-tag filter, no custom statuses, no priority or severity.** Each adds a control to every visit for a case that a tag or a second visit handles.
- **No platform, version or date filters in the main bar for now.** Five want them; they need backend and more chrome. Wait for someone to ask twice. Dates belong in the export flow if anywhere.
- **No day-group headers.** More rows of chrome in a list that is about to show more message per row.
- **No four-option segmented status control.** It would replace the one-primary-button pattern everywhere else; fixing the order and position of the existing actions solves the complaint.
- **No renaming of statuses** (Seen, Done). The four words are in the spec, the API, the MCP tools and the docs.
- **No skeleton loaders.** Delay the loading text by 300 ms and keep the previous results dimmed while searching.
- **No email-in-message hint.** The app already warns on email-shaped user IDs, which is the privacy promise; message content is the customer's own text.
- **No dark mode, no AI inside the UI, no dashboards or charts, no Baxter speech or motion.** All protected by the spec and by the reviewers.

## Decisions the reviewers asked for

- Auto-advance after a decision: on by default. Undo makes it safe.
- Undo writes a second history entry rather than erasing one.
- Counts under filters come from the backend, not hiding.
- Search with a query spans all statuses and shows that it does.
- Mobile is a supported reading and triage surface; bulk stays desktop.
- The detail page may drop its visible title; the spec gets that one exception.
- Reviewed means read and still open; Actioned means something was done about it; "Mark new" reopens. The copy should say so in the shortcuts dialog.

## Where it lands

All of this is UI work and goes into the public app repo (FeedBaxterFree) as the next milestone, briefed by Opus and built by Sonnet in three briefs matching the three phases. Phase 1 is roughly a week of the same cadence as M3; phases 2 and 3 are smaller. The only backend pieces are filter-aware stats, export filenames, key last-used and channel last-received, all small.
