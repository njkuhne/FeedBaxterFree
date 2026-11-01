# @feedbaxter/web

Send app feedback to a FeedBaxter server from the browser. This package is an API only: no UI, no DOM access, no dependencies, no telemetry.

## Install

```sh
npm install @feedbaxter/web
```

```ts
import { createClient } from "@feedbaxter/web";

const feedback = createClient({
  url: "https://feedback.example.com", // your FeedBaxter origin
  key: "YOUR_INGEST_KEY",              // publishable ingest key
  appVersion: "2.4.0",
});

const result = await feedback.send({
  message: "The export button does nothing.",
  userId: "cust_8f2a91",               // opaque customer ID, never an email
  metadata: { screen: "reports" },
});

if (!result.ok) {
  // result.error, result.retryable, result.queued
}
```

`send` never rejects and never throws. Every outcome is a `SendResult`.

## Script tag

The FeedBaxter binary does not serve `feedbaxter.js` in v1. Copy `node_modules/@feedbaxter/web/dist/feedbaxter.js` to your own site and load it from there.

```html
<script src="/assets/feedbaxter.js"></script>
<script>
  FeedBaxter.init({ url: "https://feedback.example.com", key: "YOUR_INGEST_KEY" });
  FeedBaxter.send({ message: "Hello" });
</script>
```

The script exposes `FeedBaxter.createClient`, `FeedBaxter.init` and `FeedBaxter.send`. It does not start by itself. `FeedBaxter.send` before `FeedBaxter.init` throws an error.

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `url` | required | FeedBaxter origin. Trailing slashes are removed. |
| `key` | required | Ingest key. |
| `appVersion` | none | Sent with each submission. |
| `platform` | `"web"` | Sent with each submission. |
| `storage` | `window.localStorage` if usable | Where the backlog lives. `null` turns the backlog off. |
| `fetch` | `globalThis.fetch` | Fetch implementation. |
| `maxAttempts` | `4` | Requests per `send` before it gives up for now. |
| `backlogLimit` | `20` | Most items kept in the backlog. 0 or less turns it off. |
| `backlogBytes` | `32768` | Most UTF-8 bytes the stored backlog may use. 0 or less turns it off. |

`createClient` throws a `TypeError` if `url` or `key` is empty or no `fetch` exists.

`send` takes `message` (required), `userId`, `metadata` (a plain object), `key` (overrides the client key for this call) and `idempotencyKey`.

## Results

```ts
type SendResult =
  | { ok: true }
  | { ok: false; error: string; retryable: boolean; queued: boolean };
```

`queued` is true only when the item is in storage at the moment `send` resolves. It will be sent later without further action.

| `error` | Cause | Retryable | What the SDK does |
| --- | --- | --- | --- |
| `invalid_payload` | Local check or server 400 | no | Reports it. |
| `payload_too_large` | Local check or server 413 | no | Reports it. |
| `invalid_key` | Server 401 | no | Reports it. |
| `rate_limited` | Server 429 | yes | Backs off, retries, then queues. |
| `daily_limit` | Server 429 | yes | Same. A long wait pauses the client and queues. |
| `plan_limit` | Server 429 | no | Stops sending for this client instance and queues. |
| `queue_full` | Server 503 | yes | Backs off, retries, then queues. |
| `network_error` | Fetch failed or took over 15 seconds | yes | Backs off, retries, then queues. |
| `server_error` | Other 5xx, or 408 | yes | Backs off, retries, then queues. |
| `http_error` | Any other non-202 status, such as 404 from a wrong `url` | no | Reports it. |
| `internal` | Unexpected error in the SDK | no | Reports it. |

## Retries and backoff

A retryable failure waits `min(30 s, 1 s * 2^(attempt-1))` with jitter, and at least as long as a `Retry-After` header (whole seconds) when the server sends one. If a wait would exceed 60 seconds, `send` resolves at once with `queued: true` and the item waits in the backlog. For `rate_limited` and `daily_limit` the client then pauses: later sends go straight to the backlog until the wait is over.

## Backlog

- It lives in `localStorage` under `feedbaxter.pending.v1`, oldest first, capped by `backlogLimit` and `backlogBytes`. The oldest items are dropped first.
- An item is written on its first retryable failure. A send that succeeds first time writes nothing.
- It is flushed shortly after `createClient`, when the browser fires `online`, and on every `send`. There is no background timer.
- `plan_limit` stops sending for the life of the client instance. A new page load tries again.
- Storage that is missing, blocked or full is tolerated: the SDK works without a backlog and `queued` is false.
- In Node there is no `window`, so there is no storage and no backlog. Retries still work in memory.
- Several tabs share one backlog without locking. A rare race can send an item twice; the server deduplicates by idempotency key.

## Idempotency

Each `send` gets one idempotency key (a UUID, or your `idempotencyKey`). It is reused on every retry and every backlog flush, and the server stores one entry per key. Without `crypto.randomUUID` the SDK uses `crypto.getRandomValues`; with no Web Crypto at all it falls back to a weaker `Math.random` based key. That only matters if two keys collide inside one tenant.

## Limits

Checked locally before any request, and never retried:

- `message`: 5000 characters (code points), not blank. Never truncated.
- `metadata`: a plain object of at most 4096 bytes as UTF-8 JSON.
- Whole request body: 16384 bytes.
- `idempotencyKey`: 1 to 128 characters, no control characters.

A server configured with lower limits answers 400 or 413, which the SDK reports the same way.

## Privacy

The SDK sends what you give it to `url`, and nothing else. It reads no cookies and sends none, and it does not read the page, URL, referrer, screen or time zone. It has no telemetry and writes no logs. `userId` must be an opaque customer ID, never an email address. The ingest key is publishable: it is safe to ship in client code and can only create feedback.

## License

MIT, see `LICENSE`.
