# M3 brief D: packaging, CI, self-host compose, docs

Author: Opus director · Milestone M3 · Sources: ARCHITECTURE §13, `docs/design/m3-ui-design.md` §8, brief A (web/ layout, `make ui`).

Briefs B and C run in parallel and only touch `web/src`. You touch only the files below. The UI already builds (`cd web && npm run build` → `web/dist`), and the binary embeds `web/dist` via `web/embed.go` (`//go:embed all:dist`, with `web/dist/.gitkeep` committed so Go builds without a UI build).

## Files

`deploy/Dockerfile`, `.dockerignore`, `deploy/docker-compose.yml`, `.github/workflows/ci.yml`, `Makefile` (image only, plus help text), `README.md`, `docs/OPS.md`, `.env.example` (only if a new variable is needed; none is expected).

## Dockerfile

Add a first stage and remove the TODO:
```dockerfile
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build
```
In the Go stage, after `COPY . .`: `COPY --from=ui /web/dist ./web/dist` (replaces whatever came from the context). `--platform=$BUILDPLATFORM` keeps the node stage native when building for another arch (the output is arch-independent). Keep everything else (scratch, non-root 65532, `/data` volume, ca-certificates). Pin `node:22-alpine` as written (major tag; the brief allows it).

## .dockerignore

Keep the existing entries and add `web/dist` (the node stage produces it) and `web/node_modules` (already covered by `**/node_modules`, keep explicit for clarity), `data-*`. Do not ignore `web/` itself. Check that `assets/` being ignored is fine (the UI must not import from `../assets`; brief A copied the marks into `web/`).

## docker-compose.yml (self-host)

Publish `"${FEEDBAXTER_PORT:-8080}:8080"` so a host where 8080 is taken can run it (this machine: use `FEEDBAXTER_PORT=8095`). Add `stop_grace_period: 30s` (OPS says at least 20 s) and a healthcheck is not possible in scratch (no shell or curl): do not add one. Keep the `.env` file optional. Document `FEEDBAXTER_PORT` in a comment in the compose file and in `.env.example` near the top (compose reads `.env` from the project directory only if invoked there; with `-f deploy/docker-compose.yml` from the repo root the variable comes from the shell, so the README shows `FEEDBAXTER_PORT=8095 docker compose …`).

## Makefile

`image` stays `docker build -f deploy/Dockerfile …` (the node stage is inside the Dockerfile). Update the `help` comment of `image` to "Build the scratch Docker image (UI included)". Do not touch `ui`, `dev-ui`, `dev`.

## CI (.github/workflows/ci.yml)

- New job `ui`: checkout, `actions/setup-node@v4` with `node-version: 22`, `cache: npm`, `cache-dependency-path: web/package-lock.json`; `cd web && npm ci && npm run typecheck && npm test && npm run build`.
- In `vet`, `test`, `test-pg`, `size-check`: add the same setup-node step and `make ui` before the Go step, so the measured and tested binary embeds the real UI.
- `image` job unchanged (the Dockerfile builds the UI). Upload no artifacts.

## Self-host verification from scratch (this Mac; Docker 29, compose v5)

```
docker compose -f deploy/docker-compose.yml down -v      # only this project's volume
FEEDBAXTER_PORT=8095 docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml logs feedbaxter | grep -E "ingest key|temporary password"
curl -sI http://localhost:8095/ ; curl -s http://localhost:8095/healthz ; curl -s http://localhost:8095/readyz
```
Then sign in at http://localhost:8095 in the Browser pane (mcp__Claude_Browser__* tools) with the logged temporary password, change it, and send one feedback with curl and the logged ingest key; confirm it appears in the inbox (or, if brief B is not finished yet, in `GET /api/feedback` via the browser session). Paste the image size (`docker image ls feedbaxter:latest`). Finally `docker compose -f deploy/docker-compose.yml down -v` to leave nothing running. Never touch other containers on this machine (ports 3000, 3003, 5174, 7101, 8025, 8080 belong to other projects).

## README.md

Replace the outdated quick start. New content, plain and short:
1. `docker compose -f deploy/docker-compose.yml up -d --build` (with the `FEEDBAXTER_PORT` note).
2. `docker compose -f deploy/docker-compose.yml logs feedbaxter` shows two lines once: the Default channel's ingest key and the admin's temporary password.
3. Open `http://localhost:8080`, sign in as `admin@localhost` (or `ADMIN_EMAIL`) with the temporary password, choose a new password.
4. The inbox shows "Send your first feedback" with a ready curl command; send one and it appears.
5. Lost the key: create a new one under Channels and keys. Lost the password: `docker compose exec feedbaxter /feedbaxter reset-admin`.
Then "Develop the UI": `make dev` (Go on :8090) and `make dev-ui` (Vite on :5173 with a proxy), `make ui` to build `web/dist` before `make build`. Keep the existing Build and test list and add `make ui`, `make dev-ui`.

## docs/OPS.md "First boot"

Add after the two log lines: the UI flow (sign in, forced change, onboarding page with the curl command, "Create a key" if the log is lost). Mention that the UI is served by the same binary at `/`, makes no external requests (fonts are bundled), and that hashed files under `/assets/` are cached for a year while `index.html` is never cached, so upgrades take effect on the next page load. Keep it factual and short.

## Acceptance

- `make image` succeeds; paste the tail and `docker image ls feedbaxter:latest`.
- The from-scratch compose run above, with the pasted log lines (the password and key may be shown; they are throwaway), the curl outputs and what you saw in the browser.
- `make size-check` after `make ui`; paste the sizes.
- CI file is valid YAML: check with `ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml")'` (Ruby ships with macOS) or a careful read; say which.
- No git commands of any kind. Stop and remove everything you started.
