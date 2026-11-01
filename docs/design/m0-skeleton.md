# M0 brief: skeleton (module, `feedbaxter`, `ext`, `cmd/feedbaxter`)

Author: Opus director · Milestone M0 · Source: ARCHITECTURE §2, §3, §12

## Goal

The module compiles, exposes the binding extension surface from ARCHITECTURE §3, and ships a CLI whose
`migrate` works (wired in phase B, after config and store exist). Nothing from M1 (queue, ingest,
processor, HTTP) is started.

## Phase A (coder 1): files

| File | Contents |
| --- | --- |
| `go.mod`, `go.sum` | `module github.com/njkuhne/feedbaxter`, `go 1.27` exactly (use `go mod edit -go=1.27` if `go mod init`/`go get` writes a patch version or bumps it). All six allowed dependencies added with `go get` at the **package** paths later code imports, so go.sum is complete for M0–M1 (see "Dependencies"). |
| `feedbaxter.go` | package `feedbaxter`: `Version`, `Options`, `Run`, `Migrate`, `ErrNotImplemented`. |
| `ext/ext.go` | package `ext`: the types below. Standard library imports only. |
| `cmd/feedbaxter/main.go` | CLI, standard `flag` package only. |

### Dependencies (go get exactly these; never run `go mod tidy` during M0)

```
github.com/nats-io/nats-server/v2/server@v2.15.2
github.com/nats-io/nats.go/jetstream@v1.55.0
modernc.org/sqlite@v1.61.1
github.com/jackc/pgx/v5/stdlib@v5.12.0
golang.org/x/crypto/argon2@latest
github.com/modelcontextprotocol/go-sdk/mcp@v1.8.0
```
Unimported ones will be marked `// indirect`; that is expected and fine for M0. `go mod tidy` would
drop them, which is why it is banned until M1 owns go.mod. No other module may appear as a direct
requirement.

### `ext` exported API (binding shape from ARCHITECTURE §3)

```go
package ext

type Tenant struct { ID, Name string; BillingMonthStart int /* day of month, 1..28 */ }
type TenantResolver interface {
    ByID(ctx context.Context, id string) (Tenant, error)
    Default(ctx context.Context) (Tenant, error)
}
type Authenticator interface {
    Mount(mux *http.ServeMux, deps Deps)
    Principal(r *http.Request) (Principal, bool)
}
type Role string
const ( RoleAdmin Role = "admin"; RoleReviewer Role = "reviewer" )
type Principal struct { UserID, TenantID string; Role Role; Via string /* "ui" | "mcp" */ }
type LimitsSource interface { Limits(ctx context.Context, tenantID string) (Limits, error) }
type Limits struct{ Daily, Monthly int64 } // 0 = unlimited

// Deps is what extensions receive. M0 holds only what exists; M1 adds the NATS connection
// and the KV key writer, M2 may add more. Exported types only, never internal ones.
type Deps struct {
    DB      *sql.DB
    Dialect string // "sqlite" | "postgres"
    Logger  *slog.Logger
}

// Migration is an extension migration, applied after core migrations in its own version
// namespace. Version must start with "ext_" (e.g. "ext_0001_plans"). Both SQL bodies are required.
type Migration struct { Version, SQLite, Postgres string }
```
Every exported identifier gets a doc comment. No methods beyond these, no extra types.

### `feedbaxter` exported API

```go
package feedbaxter

var Version = "dev" // set by -ldflags "-X github.com/njkuhne/feedbaxter.Version=<v>"

var ErrNotImplemented = errors.New("not implemented")

type Options struct {
    Mode           string // overrides MODE when non-empty
    TenantResolver ext.TenantResolver
    Authenticator  ext.Authenticator
    LimitsSource   ext.LimitsSource
    Migrations     []ext.Migration
    MountRoutes    func(mux *http.ServeMux, deps ext.Deps)
    OnStart        []func(ctx context.Context, deps ext.Deps) error
}

// Run starts the configured mode and blocks until ctx is done.
func Run(ctx context.Context, opts Options) error
// Migrate applies core migrations then opts.Migrations, and returns the versions it applied.
func Migrate(ctx context.Context, opts Options) ([]string, error)
```
Phase A behaviour: `Run` validates `opts.Mode` (empty or one of `ingest|worker|api|all`, else an error
naming the bad value) and returns `fmt.Errorf("serve: %w until M1", ErrNotImplemented)`. `Migrate`
returns `fmt.Errorf("migrate: %w yet", ErrNotImplemented)` (replaced in phase B).

### CLI (`cmd/feedbaxter/main.go`)

```
feedbaxter serve [--mode=ingest|worker|api|all]   (flag default "": MODE env decides)
feedbaxter migrate
feedbaxter reset-admin
feedbaxter version                                 prints "feedbaxter <Version>"
feedbaxter help | -h | --help                      usage to stdout, exit 0
```
- Root context: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`.
- `main` calls `run(args []string, stdout, stderr io.Writer) int` so it is testable.
- Exit codes: 0 success, 1 runtime error (message `feedbaxter: <err>` on stderr), 2 usage error
  (unknown subcommand, bad flag, missing subcommand; usage printed on stderr).
- `reset-admin` returns `reset-admin: not implemented until M2` with exit 1.
- `serve` builds `feedbaxter.Options{Mode: *mode}` and calls `Run`. The public cmd fills no
  extensions yet (single-tenant resolver and self-host auth arrive in M1/M2).
- `migrate` calls `feedbaxter.Migrate(ctx, feedbaxter.Options{})` and prints one line per applied
  version (`applied 0001_core`) or `schema is up to date`.

### Tests (phase A)

`cmd/feedbaxter/main_test.go`:
- `TestVersion` – `run([]string{"version"})` exits 0, prints `feedbaxter dev`.
- `TestUnknownCommand`, `TestNoCommand` – exit 2, usage on stderr.
- `TestServeBadMode` – `serve --mode=nope` exits 1 and stderr names `nope`.
- `TestResetAdminNotImplemented` – exits 1, stderr mentions M2.

`feedbaxter_test.go`: `TestRunNotImplemented` – `errors.Is(err, ErrNotImplemented)` for mode `all`;
`TestRunRejectsBadMode`.

## Phase B (coder 4): wire `migrate` and config into the root package

- `Run`: `config.Load(overrides)` where overrides is `{"MODE": opts.Mode}` when non-empty; return the
  config error if any; otherwise still `ErrNotImplemented` until M1. The mode check from phase A moves
  to config (one place).
- `Migrate`: `config.Load` → `store.Open(ctx, cfg.Database)` → `store.Migrate(ctx, db, opts.Migrations)`
  → close → return applied versions. Logs with the config's logger at info.
- Tests (`feedbaxter_test.go`): `TestMigrateSQLite` – `t.Chdir(t.TempDir())`, `t.Setenv("DATABASE_URL",
  "sqlite://"+dir+"/fb.db")`; first call returns `["0001_core"]`, second returns empty; and
  `TestMigrateWithExtension` – an `ext_0001_demo` migration is applied after core and listed.
- CLI test `TestMigrateCommand` – same setup through `run([]string{"migrate"})`, prints
  `applied 0001_core`, second run prints `schema is up to date`.

## Acceptance

1. `make vet` and `make test` green; `go list -m all` shows only the six direct modules plus transitive.
2. `ext` imports only the standard library; nothing in the tree references a private repo.
3. `bin/feedbaxter version` prints the version injected by `make build`.
4. After phase B, `feedbaxter migrate` against a fresh SQLite path creates the schema; a second run
   applies nothing.

Out of scope: any HTTP server, NATS, single-tenant resolver, self-host auth, `reset-admin` logic (M1/M2).
