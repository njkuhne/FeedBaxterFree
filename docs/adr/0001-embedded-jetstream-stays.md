# ADR 0001: Keep embedded NATS JetStream for self-host

Date: 2026-11-01 · Status: accepted

## Context
The spec wanted a 15–25 MB binary idling at 20–30 MB RAM and an embedded NATS server with JetStream. The two looked incompatible.

## Decision
Keep embedded JetStream in `all` mode. Set the footprint targets to binary ≤ 32 MB and idle RSS ≤ 48 MB, checked in CI.

## Evidence
Probe in `docs/probe/` (Go 1.27.3, `CGO_ENABLED=0`, `-s -w -trimpath`, arm64): 19.5 MB binary with nats-server 2.15 + JetStream + nats.go + modernc SQLite; 27 MB idle RSS with a stream, consumer and SQLite open; 214k msg/s drained into SQLite in batches of 500.

## Consequences
One queue code path for self-host and SaaS. Self-host needs a persistent volume for the JetStream store as well as the SQLite file (it needed one anyway). The alternative, a SQLite-backed queue for self-host only, is rejected: it would save ~10 MB binary and ~15 MB RAM for a second implementation to test.
