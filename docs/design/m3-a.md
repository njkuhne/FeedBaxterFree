# M3 brief A: UI foundation, shell, auth screens, Go serving

Author: Opus director · Milestone M3 · Design source: `docs/design/m3-ui-design.md` (read it all; it is binding). API: `docs/API.md`.

## Scope

1. `web/` project: Vite + React 19 + TypeScript strict + react-router-dom 7 (library mode, `createBrowserRouter`), Vitest with jsdom and @testing-library/react (use `fireEvent`; no user-event package).
2. Tokens, base CSS, fonts, icons, Baxter components, every shared component in design §3.
3. The shell (rail/top bar, skip link, routing, auth gate, focus on route change, document titles), toasts, announcer, dialog primitive, API client.
4. Screens: Login, Change password (forced), Profile, Not found. Placeholder pages for every other route so later briefs only fill their own files.
5. Go: `web/embed.go`, `internal/ui`, mount in `feedbaxter.go`, tests. Makefile `ui`, `dev-ui`. `.gitignore`.

## Files

```
web/package.json  package-lock.json  tsconfig.json  vite.config.ts  index.html  embed.go  dist/.gitkeep
web/scripts/check.mjs            # source rules, see "check" below
web/public/favicon.svg           # copy of assets/brand/baxter-small.svg
web/public/fonts/OFL-Fraunces.txt  web/public/fonts/OFL-Inter.txt
web/src/main.tsx  App.tsx  routes.tsx
web/src/styles/tokens.css  base.css  fonts.css  components.css  layout.css
web/src/fonts/*.woff2            # copied from the fontsource packages, latin only, committed
web/src/lib/api.ts endpoints.ts types.ts time.ts format.ts email.ts filters.ts cursorList.ts shortcuts.ts
web/src/lib/*.test.ts
web/src/components/Button.tsx TextField.tsx SelectField.tsx Checkbox.tsx Segmented.tsx StatusMarker.tsx
  StatusChip.tsx TagPill.tsx Toast.tsx Announcer.tsx Dialog.tsx ConfirmDialog.tsx EmptyState.tsx CodeBlock.tsx
  SecretOnce.tsx EmailWarning.tsx InlineAlert.tsx icons.tsx Baxter.tsx VisuallyHidden.tsx
web/src/components/*.test.tsx    # Dialog, Toast, StatusChip, Checkbox (see tests)
web/src/shell/AuthProvider.tsx Shell.tsx Rail.tsx usePageTitle.ts
web/src/pages/Login.tsx ChangePassword.tsx Profile.tsx NotFound.tsx
web/src/pages/Inbox.tsx FeedbackDetail.tsx PerUser.tsx Channels.tsx People.tsx Settings.tsx   # placeholders for briefs B and C: render only the page <h1>
internal/ui/ui.go  internal/ui/ui_test.go
```
Change: `feedbaxter.go` (mount), `feedbaxter_test.go` (one test that `/` serves HTML in all mode), `Makefile`, `.gitignore`.

## package.json

Dependencies exactly: react, react-dom, react-router-dom. Dev: vite, @vitejs/plugin-react, typescript, @types/react, @types/react-dom, vitest, jsdom, @testing-library/react, @fontsource-variable/fraunces, @fontsource/inter. Pin exact versions (no `^`). Scripts:
- `dev`: `vite` (port 5173, `strictPort`), proxy `/api` and `/v1` to `http://localhost:8090` (changeOrigin false so cookies work; leave `Sec-Fetch-Site` alone, the client also sends `X-Requested-With`).
- `typecheck`: `tsc --noEmit`. `test`: `vitest run && node scripts/check.mjs`. `build`: `vite build && node -e "require('node:fs').writeFileSync('dist/.gitkeep','')"`.
- Vite: `build.assetsDir = "assets"`, hashed names (default), `build.target = "es2022"`, no sourcemaps in the build, `assetsInlineLimit: 0` (fonts and images as files so CSP and caching are simple).

## Fonts

Copy from node_modules into `web/src/fonts/` and reference from `fonts.css` with relative `url()` so Vite hashes them into `/assets/`:
- Fraunces: the latin file of `@fontsource-variable/fraunces` that carries the `opsz` axis with weight (inspect `files/`; prefer the `opsz` file over `full` if both carry wght+opsz; report the size). `font-weight: 100 900` range in `@font-face`, used at 400 with `font-optical-sizing: auto`.
- Inter: `inter-latin-400-normal.woff2`, `inter-latin-500-normal.woff2`.
- `font-display: swap`, `unicode-range` copied from fontsource's latin CSS. Copy each package's `LICENSE` (OFL) to `web/public/fonts/OFL-*.txt`.

## Shell and gate

`AuthProvider` loads `GET /api/me` (design §4 "App start"), exposes `{user, setUser, signOut}`; subscribes to the client's `onAuthProblem`. Routes: design §4. Admin-only routes render NotFound for reviewers. Rail per design §2 with `NavLink` (it sets `aria-current="page"`). On navigation: set `document.title`, focus the `<h1>`. `ToastProvider` and `Announcer` wrap the app. While a dialog is open, set `inert` on `#app-root` (dialogs portal to `document.body`).

## API client and endpoints

Design §7 exactly. `endpoints.ts` covers every route in API.md with typed functions (`login`, `logout`, `changePassword`, `me`, `listUsers`, `createUser`, `setRole`, `deleteUser`, `resetPassword`, `listTokens`, `createToken`, `deleteToken`, `listChannels`, `createChannel`, `renameChannel`, `deleteChannel`, `listKeys`, `createKey`, `revokeKey`, `listFeedback(filters, cursor?, limit?)`, `getFeedback`, `setStatus`, `deleteFeedback`, `bulkStatus`, `addTags`, `removeTags`, `deleteByUser`, `exportUrl(filters, format)`, `listTags`, `stats`, `ops`). Query strings via `URLSearchParams`; path segments via `encodeURIComponent`.

## Screens in this brief

Login, Change password, Profile, Not found exactly as design §4 (copy is binding, word for word). Profile token revoke and change password use ConfirmDialog/primary as specified.

## Go serving

Design §8 exactly. `web/embed.go`:
```go
// Package web embeds the built admin UI (web/dist). Without a UI build only .gitkeep is embedded.
package web
import "embed"
//go:embed all:dist
var Dist embed.FS
```
`internal/ui`: `func Handler(fsys fs.FS) http.Handler` (takes the already-`fs.Sub`'d dist). Use `http.ServeFileFS` or `io.Copy` from `fsys.Open`; set Content-Type yourself for `.woff2` (`font/woff2`), `.js` (`text/javascript; charset=utf-8`), `.css`, `.svg`, `.html`, `.txt` (Go's builtin mime table lacks woff2 and scratch has no `/etc/mime.types`). Dotfiles → 404. In `feedbaxter.go`, inside the existing `if cfg.Mode == "api" || cfg.Mode == "all"` block: `mux.Handle("/", ui.Handler(sub))` with `sub, _ := fs.Sub(web.Dist, "dist")`. Register before `opts.MountRoutes` so extensions can still add more specific routes. `/healthz`, `/readyz`, `/api/`, `/v1/feedback` keep winning because they are more specific patterns.

Go tests (`internal/ui/ui_test.go`, using `fstest.MapFS`): `TestAssetsImmutable`, `TestMissingAssetIs404`, `TestSPAFallbackNoStore` (`/inbox/x` → index.html, `no-store`), `TestRootFilesNoCache` (`/favicon.svg`), `TestReservedPrefixesNotSPA` (`/v1/x`, `/mcp`, `/api` → 404 JSON), `TestDotfile404`, `TestMethodNotAllowed` (POST `/` → 405), `TestWoff2ContentType`, `TestSecurityHeaders` (CSP and nosniff on index and assets), `TestNotBuilt` (MapFS with only `.gitkeep` → 503 text). In `feedbaxter_test.go`: in all mode `GET /` answers 200 or 503 (depending on whether dist is built) with the CSP header, and `GET /api/me` is still the API's 401 JSON.

## Makefile and .gitignore

```make
ui: ## Build the admin UI into web/dist (needs Node 22+)
	cd web && npm ci --no-audit --no-fund && npm run build
dev-ui: ## Vite dev server on localhost:5173, proxying /api and /v1 to make dev (:8090)
	cd web && npm run dev
```
Add both to `.PHONY`; remove the "ui target arrives in M3" comment. `.gitignore`: replace `/web/dist/` with `/web/dist/*` and `!/web/dist/.gitkeep`.

## check (web/scripts/check.mjs, Node standard library only)

Walk `web/src` and fail with file:line on: a hex colour, `rgb(`/`rgba(`/`hsl(` outside `styles/tokens.css`; `dangerouslySetInnerHTML`; `innerHTML`; the regex `:\s*any\b|as any\b|<any>`; `http://` or `https://` in non-test files (the curl example builds its URL from `location.origin`); `font-family` outside `tokens.css`/`base.css`/`fonts.css` that is not `var(--font-…)`; `outline:\s*none` and `outline:\s*0`.

## Tests (Vitest)

- `api.test.ts`: sends `X-Requested-With: fetch` on every method; JSON body sets Content-Type; 204 returns undefined; error body → ApiError with code/message; non-JSON error → internal; network failure → code network; 401 and `password_change_required` call the subscriber, other 403s do not.
- `time.test.ts`: each bucket of `relativeTime` with a fixed `now`, year boundary, `absoluteTime` format.
- `email.test.ts`: `a@b.co` yes; `cust-4821`, `usr@local`, `a @b.co`, `""` no; surrounding spaces trimmed.
- `filters.test.ts`: round-trip query ⇄ filters; default status new omitted from the URL; "all" sends no status; empty values dropped.
- `cursorList.test.ts`: appends and dedupes; stale request ignored; reset clears; remove ids; nextCursor null ends.
- `shortcuts.test.ts`: j/k/x/e/a/i/n/Enter/Escape///? mapping; ignored in inputs, with modifiers, or while a dialog is open.
- `format.test.ts`: bytes, duration, plural.
- `Dialog.test.tsx`: opens with focus on Cancel (destructive), Tab cycles inside, Escape closes, focus returns to the opener, `aria-modal` and labelledby present.
- `Toast.test.tsx`: message lands in the polite region; error in the alert region; auto-dismiss with fake timers.
- `StatusChip.test.tsx`: each status renders its word.

## Acceptance

- `cd web && npm run typecheck && npm test && npm run build` pass; paste outputs. `make vet` and `make test` pass; paste the tail.
- `make ui && make dev`, then http://localhost:8090: login with the first-boot password from the dev log works, the forced change works, the rail shows, Profile works (password, tokens), `/nonsense` shows Not found, `/assets/*` has the immutable header (curl -I), index has `no-store`. Paste the curl -I output.
- No runtime request leaves the origin. JS gzip size reported (`npm run build` prints it).
- No git commands of any kind (no add, commit, push, branch, worktree, stash).
