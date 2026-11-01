# ADR 0003: The ingest key cache is a NATS KV watch

Date: 2026-11-01 · Status: accepted

## Context
The spec synced the key cache three ways: NATS core events, a 5-minute full resync, and a synchronous fallback lookup to the BFF on a cache miss. Core publish is at-most-once, so a lost revoke stays live for up to 5 minutes, and the fallback made ingest depend on the API process.

## Decision
One KV bucket `keys` keyed by `sha256(key)`, written by the API on every key or limit change, watched by ingest with history replay on startup (which also gates readiness). The 5-minute job is a DB → KV reconcile in the API. Unknown key means `invalid_key`; ingest never calls the API.

## Consequences
No lost events (KV is JetStream-backed), no negative cache, no runtime coupling between ingest and api. Dual-write (DB then KV) is reconciled by the periodic job. The counters bucket uses the same mechanism with compare-and-swap merges.
