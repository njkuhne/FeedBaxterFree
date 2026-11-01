# FeedBaxter

FeedBaxter captures one-way text feedback from web, iOS and macOS apps through an API, and gives
teams a simple inbox to triage it. It runs as a free, open-source, single-tenant self-hosted app
or as a paid multi-tenant SaaS. No AI runs inside it; triage happens through the MCP server with
your own AI client.

## Self-host quick start

```sh
docker compose -f deploy/docker-compose.yml up -d --build
```

Then open `http://localhost:8080`. The server lands in M1 and the admin UI in M3, so today the
image builds and `migrate` runs, but nothing answers on that port yet.

Configuration is by environment variables; copy `.env.example` to `.env` to change them.

## Build and test

Go runs inside the official `golang` Docker image, so only Docker and make are needed.

```sh
make build       # bin/feedbaxter plus linux amd64 and arm64 binaries
make test        # test suite on SQLite
make test-pg     # test suite against Postgres (starts and removes a container)
make vet         # go vet
make image       # scratch Docker image, feedbaxter:latest
make size-check  # fail if a linux binary exceeds 32 MiB
```

Run `make help` for the full list.

## Documentation

- [Product spec](docs/SPEC.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Decision records](docs/adr/)
- [Component design briefs](docs/design/)

## License

AGPL-3.0, see `LICENSE`.
