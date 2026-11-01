# ADR 0005: No client IP is collected or stored

Date: 2026-11-01 · Status: accepted

## Decision
The queue envelope carries tenant, channel, key ID, received time and idempotency key only. No table has an IP column. Rate limiting is per ingest key, in memory.

## Why
Nothing used the IP, and holding personal data nothing uses contradicts the product's privacy-by-design stance and complicates GDPR. If abuse control ever needs per-IP limiting, it happens in memory at ingest and is never written.
