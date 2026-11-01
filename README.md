# FeedBaxter

FeedBaxter captures one-way text feedback from web, iOS and macOS apps through an API, and gives
teams a simple inbox to triage it. It runs as a free, open-source, single-tenant self-hosted app
or as a paid multi-tenant SaaS. No AI runs inside it; triage happens through the MCP server with
your own AI client.

## Self-host quick start

1. Start it. If host port 8080 is taken, set `FEEDBAXTER_PORT` (for example `FEEDBAXTER_PORT=8095`) in front of the command.

   ```sh
   docker compose -f deploy/docker-compose.yml up -d --build
   ```

2. Read the first-boot log. It shows two lines, once: the Default channel's ingest key and the admin's temporary password.

   ```sh
   docker compose -f deploy/docker-compose.yml logs feedbaxter
   ```

3. Open `http://localhost:8080`, sign in as `admin@localhost` (or your `ADMIN_EMAIL`) with the temporary password, and choose a new password.
4. The inbox shows "Send your first feedback" with a ready curl command. Send one and it appears.
5. Lost the key: create a new one under Channels and keys. Lost the password: `docker compose -f deploy/docker-compose.yml exec feedbaxter /feedbaxter reset-admin`.

Configuration is by environment variables; copy `.env.example` to `.env` to change them.

## Develop the UI

```sh
make dev      # Go server on :8090
make dev-ui   # Vite on :5173, proxying /api and /v1 to :8090
make ui       # build web/dist; run it before make build so the binary embeds the UI
```

## Build and test

Go runs inside the official `golang` Docker image, so Go itself is not needed; building the UI needs Node 22 or later.

```sh
make ui          # build the admin UI into web/dist
make dev-ui      # Vite dev server on :5173
make build       # bin/feedbaxter plus linux amd64 and arm64 binaries
make test        # test suite on SQLite
make test-pg     # test suite against Postgres (starts and removes a container)
make vet         # go vet
make image       # scratch Docker image, feedbaxter:latest
make size-check  # fail if a linux binary exceeds 32 MiB
make rss-check   # boot the image, sign in, fail if RSS exceeds 48 MiB
```

Run `make help` for the full list.

## Documentation

- [Product spec](docs/SPEC.md)
- [Architecture](docs/ARCHITECTURE.md)
- [MCP server](docs/MCP.md)
- [Web SDK](sdk/web/README.md)
- [Decision records](docs/adr/)
- [Component design briefs](docs/design/)

## License

AGPL-3.0, see `LICENSE`.

The Web SDK in `sdk/web` is MIT-licensed so it can be embedded in any app; see `sdk/web/LICENSE`.
