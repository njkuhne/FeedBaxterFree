GO_IMAGE   ?= golang:1.27-alpine
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DOCKER_RUN  = docker run --rm -v "$(CURDIR)":/src -v fbx-gomodcache:/go/pkg/mod \
              -v fbx-gobuildcache:/root/.cache/go-build -w /src \
              -e CGO_ENABLED=0 -e GOTOOLCHAIN=local -e GOFLAGS=-buildvcs=false \
              -e GOOS -e GOARCH -e TEST_DATABASE_URL $(DOCKER_EXTRA) $(GO_IMAGE)
# CI sets GO=go to use a native toolchain.
GO         ?= $(DOCKER_RUN) go
LDFLAGS     = -s -w -X github.com/njkuhne/feedbaxter.Version=$(VERSION)
# 32 MiB.
SIZE_LIMIT ?= 33554432
# 48 MiB.
RSS_LIMIT  ?= 50331648
DEV_PORT   ?= 8090
# Data directory for make dev, relative to the repo (a second dev server needs its own).
DEV_DATA   ?= data
PG_URL_DEV  = postgres://feedbaxter:feedbaxter@postgres:5432/feedbaxter_test?sslmode=disable
COMPOSE_DEV = docker compose -f deploy/compose.dev.yml

HOST_GOOS   = $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOST_GOARCH = $(shell uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')

.PHONY: help build dev test test-pg vet lint image size-check clean ui dev-ui sdk-web

.DEFAULT_GOAL := help

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-12s %s\n", $$1, $$2}'

ui: ## Build the admin UI into web/dist (needs Node 22+)
	cd web && npm ci --no-audit --no-fund && npm run build

sdk-web: ## Build and test the web SDK (needs Node 22+)
	cd sdk/web && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run build

dev-ui: ## Vite dev server on localhost:5173, proxying /api and /v1 to make dev (:8090)
	cd web && npm run dev

build: ## Build bin/feedbaxter (host) and linux amd64/arm64 binaries
	GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/feedbaxter-linux-amd64 ./cmd/feedbaxter
	GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/feedbaxter-linux-arm64 ./cmd/feedbaxter
	GOOS=$(HOST_GOOS) GOARCH=$(HOST_GOARCH) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/feedbaxter ./cmd/feedbaxter

dev: DOCKER_EXTRA = --init -p $(DEV_PORT):8080 -e DATA_DIR=/src/$(DEV_DATA) -e HTTP_ADDR=:8080
dev: ## Run serve (all mode, SQLite in ./data) on localhost:DEV_PORT (default 8090; 8080 is taken on this machine)
	mkdir -p $(DEV_DATA)
	$(DOCKER_RUN) sh -c 'go build -o /tmp/feedbaxter ./cmd/feedbaxter && exec /tmp/feedbaxter serve'

test: ## Run the test suite (SQLite)
	$(GO) test -count=1 ./...

test-pg: ## Run the test suite against Postgres from deploy/compose.dev.yml
	$(COMPOSE_DEV) up -d --wait postgres || { $(COMPOSE_DEV) down -v; exit 1; }; \
	TEST_DATABASE_URL='$(PG_URL_DEV)' $(MAKE) test DOCKER_EXTRA=--network=feedbaxter-dev_default; \
	status=$$?; \
	$(COMPOSE_DEV) down -v; \
	exit $$status

vet: ## Run go vet
	$(GO) vet ./...

lint: vet ## Alias for vet

image: ## Build the scratch Docker image (UI included)
	docker build -f deploy/Dockerfile --build-arg VERSION=$(VERSION) -t feedbaxter:$(VERSION) -t feedbaxter:latest .

rss-check: ## Fail if the image's RSS, measured after a first boot and a login, exceeds RSS_LIMIT
	@c=fbx-rss-$$$$; \
	docker run -d --rm --name $$c -p 127.0.0.1:0:8080 feedbaxter:latest >/dev/null; \
	trap 'docker rm -f '$$c' >/dev/null 2>&1' EXIT; \
	port=$$(docker port $$c 8080 | head -1 | sed 's/.*://'); \
	for i in $$(seq 1 30); do curl -sf -o /dev/null http://127.0.0.1:$$port/readyz && break; sleep 1; done; \
	pw=$$(docker logs $$c 2>&1 | sed -n 's/.*temporary password: \([A-Za-z0-9]*\).*/\1/p' | head -1); \
	curl -sf -o /dev/null -H 'X-Requested-With: fetch' -H 'Content-Type: application/json' \
	  -d "{\"email\":\"admin@localhost\",\"password\":\"$$pw\"}" http://127.0.0.1:$$port/api/auth/login \
	  || { echo "rss-check: login failed"; exit 1; }; \
	sleep 5; \
	kb=$$(docker run --rm --pid=container:$$c alpine:3.21 awk '/^VmRSS/{print $$2}' /proc/1/status); \
	echo "RSS after first boot and login: $$kb kB ($$(awk -v k=$$kb 'BEGIN{printf "%.1f", k/1024}') MB)"; \
	[ "$$kb" -gt 0 ] && [ $$((kb*1024)) -le $(RSS_LIMIT) ] || { echo "RSS exceeds $(RSS_LIMIT) bytes"; exit 1; }

size-check: build ## Fail if a linux binary exceeds SIZE_LIMIT
	@status=0; \
	for f in bin/feedbaxter-linux-amd64 bin/feedbaxter-linux-arm64; do \
	  bytes=$$(wc -c < $$f | tr -d ' '); \
	  mb=$$(awk -v b=$$bytes 'BEGIN{printf "%.1f", b/1048576}'); \
	  echo "$$f: $$bytes bytes ($$mb MB)"; \
	  if [ $$bytes -gt $(SIZE_LIMIT) ]; then echo "$$f exceeds $(SIZE_LIMIT) bytes"; status=1; fi; \
	done; \
	exit $$status

clean: ## Remove build output
	rm -rf bin
