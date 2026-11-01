# ADR 0002: The private SaaS repo imports the public module; no build tags

Date: 2026-11-01 · Status: accepted

## Context
The spec had the private layer "compiled in with a Go build tag". For that, the public module would have to import the private one, so the public `go.mod` would reference a repo contributors cannot fetch.

## Decision
The public module exposes `feedbaxter.Run(ctx, Options)` and an exported `ext/` package with `TenantResolver`, `Authenticator`, `LimitsSource`, extra migrations, extra routes and startup hooks. The private repo has its own `cmd/feedbaxter-saas` that imports the public module and fills `Options`. The public tree never references the private one.

## Consequences
Extension interfaces are designed in M1–M2, not discovered in M6. Quota enforcement is public (limits are numbers on the key-cache entry); only plan management, billing, sign-up and super-admin are private. `internal/` is invisible to the private repo by design, so anything it needs must be reachable from `ext/`.
