# M0 brief: toolchain (Makefile, Docker, compose, CI, README, .env.example)

Author: Opus director · Milestone M0 · Source: ARCHITECTURE §13, CLAUDE.md "Stack"

## Goal

Go is not installed on the dev Mac. Every Go command runs in `golang:1.27-alpine` with the module cache
volume `fbx-gomodcache`. One `make` target per job; CI runs the same jobs.

## Phase A (coder 1): files

### `Makefile`

Variables (all overridable):
```make
GO_IMAGE   ?= golang:1.27-alpine
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DOCKER_RUN  = docker run --rm -v "$(CURDIR)":/src -v fbx-gomodcache:/go/pkg/mod \
              -v fbx-gobuildcache:/root/.cache/go-build -w /src \
              -e CGO_ENABLED=0 -e GOTOOLCHAIN=local -e GOFLAGS=-buildvcs=false \
              -e GOOS -e GOARCH -e TEST_DATABASE_URL $(DOCKER_EXTRA) $(GO_IMAGE)
GO         ?= $(DOCKER_RUN) go        # CI sets GO=go to use a native toolchain
LDFLAGS     = -s -w -X github.com/njkuhne/feedbaxterfree.Version=$(VERSION)
SIZE_LIMIT ?= 33554432                # 32 MiB
PG_URL_DEV  = postgres://feedbaxter:feedbaxter@postgres:5432/feedbaxter_test?sslmode=disable
```
`-e NAME` without a value forwards the host variable only when it is set, so `GOOS=linux GOARCH=arm64 $(GO) build …`
and `TEST_DATABASE_URL=… $(GO) test` behave the same with the Docker toolchain and with `GO=go`.

| Target | Does |
| --- | --- |
| `build` | `bin/feedbaxter-linux-amd64`, `bin/feedbaxter-linux-arm64`, and `bin/feedbaxter` for the host OS/arch (from `uname -s`/`uname -m`, mapped to GOOS/GOARCH: Darwin→darwin, Linux→linux, x86_64→amd64, arm64/aarch64→arm64). Each: `$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o … ./cmd/feedbaxter` prefixed with `GOOS=… GOARCH=…` (forwarded by `-e GOOS -e GOARCH`). |
| `test` | `$(GO) test -count=1 ./...` |
| `test-pg` | `docker compose -f deploy/compose.dev.yml up -d --wait postgres`, then `TEST_DATABASE_URL='$(PG_URL_DEV)' $(MAKE) test DOCKER_EXTRA=--network=feedbaxter-dev_default`, then `docker compose … down -v` **whether or not tests failed**, exiting with the test status (one shell recipe: `…; status=$$?; docker compose … down -v; exit $$status`). |
| `vet` | `$(GO) vet ./...` (`lint` is an alias, as ARCHITECTURE §13 names it) |
| `image` | `docker build -f deploy/Dockerfile --build-arg VERSION=$(VERSION) -t feedbaxter:$(VERSION) -t feedbaxter:latest .` |
| `size-check` | depends on `build`; for each linux binary prints `<file>: <bytes> bytes (<MB with one decimal>)` and fails if any exceeds `SIZE_LIMIT`. Use `wc -c` (portable on macOS and Linux). |
| `clean` | `rm -rf bin` |
| `help` | default target; lists targets with one-line descriptions. |

All targets `.PHONY`. `ui` and `dev` from ARCHITECTURE §13 are **not** added in M0 (no `web/`, no
pipeline to run); leave a comment saying they arrive in M3 and M1.

### `deploy/Dockerfile`

```
# syntax=docker/dockerfile:1
# TODO(M3): add a node stage that builds web/ into web/dist before the Go build.
FROM golang:1.27-alpine AS build
```
- `apk add --no-cache ca-certificates`; `WORKDIR /src`; copy `go.mod go.sum`, `go mod download` (with a
  `--mount=type=cache,target=/go/pkg/mod`); copy the source; build with `CGO_ENABLED=0 GOTOOLCHAIN=local`,
  `-trimpath -ldflags "-s -w -X …Version=${VERSION}"`, `GOOS=$TARGETOS GOARCH=$TARGETARCH` (declare
  `ARG TARGETOS TARGETARCH VERSION=dev`), output `/out/feedbaxter`.
- `RUN mkdir -p /out/data && chown 65532:65532 /out/data` (scratch has no shell to do it later).
- Final `FROM scratch`: copy `/etc/ssl/certs/ca-certificates.crt`, the binary to `/feedbaxter`, and
  `/out/data` to `/data` with `--chown=65532:65532`. `ENV DATA_DIR=/data`, `USER 65532:65532`,
  `VOLUME ["/data"]`, `EXPOSE 8080`, `ENTRYPOINT ["/feedbaxter"]`, `CMD ["serve"]`.
  (ENTRYPOINT is the binary and CMD is `serve`, so `docker compose exec feedbaxter /feedbaxter reset-admin`
  and `docker run … migrate` work.)
- `.dockerignore` at the root: `.git`, `bin`, `data`, `docs`, `assets`, `**/node_modules`, `.env`, `.env.*`.

### `deploy/docker-compose.yml` (self-host)

One service `feedbaxter`: `build: {context: .., dockerfile: deploy/Dockerfile}`, `image: feedbaxter:latest`,
`ports: ["8080:8080"]`, `volumes: ["feedbaxter-data:/data"]`, `env_file: [{path: ../.env, required: false}]`,
`restart: unless-stopped`. Named volume `feedbaxter-data`. No healthcheck yet (scratch has no shell; M1
adds `/healthz` and decides).

### `deploy/compose.dev.yml` (tests and M1 dev)

`name: feedbaxter-dev`. `postgres: image postgres:16-alpine`, env `POSTGRES_USER=feedbaxter`,
`POSTGRES_PASSWORD=feedbaxter`, `POSTGRES_DB=feedbaxter_test`, `ports: ["5433:5432"]`, `tmpfs:
/var/lib/postgresql/data`, healthcheck `pg_isready -U feedbaxter -d feedbaxter_test` every 1s, 30 retries.
`nats: image nats:2.11-alpine`, `command: ["-js", "-sd", "/data"]`, tmpfs `/data`, **no published ports**.

### `.env.example`

Every variable from ARCHITECTURE §11 with its default, grouped by mode with a one-line `#` comment
above each group, in §11 order. `DATABASE_URL` and `NATS_URL` lines are commented out with the default
behaviour explained. No inline comments (the `.env` parser does not support them).

### `README.md`

Three sentences on what FeedBaxter is (from SPEC Overview). Self-host quick start:
`docker compose -f deploy/docker-compose.yml up -d --build`, then open `http://localhost:8080` (state
plainly that the server lands in M1 and the UI in M3; today the image builds and `migrate` runs).
Build and test: `make build`, `make test`, `make test-pg`, `make vet`, `make image`, `make size-check`,
noting Go runs in Docker and only Docker + make are needed. Links to `docs/SPEC.md`,
`docs/ARCHITECTURE.md`, `docs/adr/`, `docs/design/`. License line: AGPL (license file is Fable's call;
say "AGPL-3.0 (license file to follow)"). Under 80 lines.

## Phase B (coder 4): `.github/workflows/ci.yml`

Triggers: push to main, pull_request. `permissions: contents: read`. Jobs, each on `ubuntu-latest` with
`actions/checkout@v4` and `actions/setup-go@v5` (`go-version-file: go.mod`, cache on), running make with
`GO=go` so CI uses the native toolchain and the same targets:
- `vet` – `make GO=go vet`.
- `test` – `make GO=go test`.
- `test-pg` – `services.postgres` (`postgres:16-alpine`, same user/password/db as compose.dev, port
  5432:5432, health-cmd `pg_isready`), env
  `TEST_DATABASE_URL=postgres://feedbaxter:feedbaxter@localhost:5432/feedbaxter_test?sslmode=disable`,
  run `make GO=go test`.
- `size-check` – `make GO=go size-check`.
- `image` – `docker/setup-buildx-action@v3`, then `make image`.
Set `env: CGO_ENABLED: "0"` at workflow level. Validate the YAML locally by running it through a YAML
parser in a container (e.g. `python:3-alpine` with `yaml` module, or `docker run --rm -v "$PWD":/w
rhysd/actionlint:latest -color` if available) and paste the result.

## Acceptance

1. `make vet`, `make test`, `make build`, `make size-check`, `make image` succeed on the Mac with only
   Docker installed; `make test-pg` starts Postgres on 5433, runs the suite, and leaves no containers
   running (`docker ps` shows no `feedbaxter-dev` containers afterwards), also when a test fails.
2. Linux binaries are static (`file` or `go version -m` shows CGO_ENABLED=0) and each ≤ 32 MiB.
3. `docker run --rm feedbaxter:latest version` prints the version; image runs as uid 65532.
4. No published NATS port; Postgres only on host port 5433.
