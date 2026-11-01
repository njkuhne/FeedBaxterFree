# ADR 0006: MCP over Streamable HTTP with personal access tokens

Date: 2026-11-01 · Status: accepted

## Decision
The MCP server is mounted inside the api mode at `/mcp` using the official `modelcontextprotocol/go-sdk` with the Streamable HTTP transport. Auth is `Authorization: Bearer <personal access token>`; the token's user and role are the principal, and tools call the same service functions the JSON API uses.

## Why
The server is remote (self-hosters' AI clients run on laptops, not in the container), so stdio is not an option. Reusing the service layer keeps permission checks and `via: mcp` history in one place.
