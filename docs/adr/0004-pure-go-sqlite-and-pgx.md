# ADR 0004: modernc.org/sqlite and pgx; hand-written SQL with a minimal dialect shim

Date: 2026-11-01 · Status: accepted

## Decision
SQLite via `modernc.org/sqlite` (pure Go) so the binary builds with `CGO_ENABLED=0` into a `scratch` image and cross-compiles trivially. Postgres via `pgx/v5` in `database/sql` mode. No ORM and no query generator; hand-written queries and a dialect shim limited to placeholder style, JSON column type and the current-timestamp function. IDs are prefixed random strings in `TEXT` columns; timestamps are Unix milliseconds in `INTEGER` columns, in both databases.

## Consequences
Pure-Go SQLite is ~1.5–2× slower per statement than CGO SQLite; measured at 214k batched inserts/s it does not matter. Every store test runs against both dialects. No separate schemas (SQLite has none).
