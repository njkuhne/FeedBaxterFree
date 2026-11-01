# M5 brief (part 1): `sdk/web`, the Web SDK

Author: Opus director · Milestone M5 part 1 · Source: SPEC "Ingest API and SDKs" and "Rejection responses";
ARCHITECTURE §2, §4, §11; `internal/ingest/handler.go` and `errors.go` (the wire contract); REVIEW §5 (M5)

An npm package and a script-tag build that post feedback to `POST /v1/feedback`. API only: no UI, no
DOM, no FeedBaxter branding, no runtime dependencies, no telemetry. **Nothing in Go, `web/` or the ingest
contract changes.** If the implementation seems to need a contract change, stop and report it.

## Files

```
sdk/web/package.json         package manifest (below)
sdk/web/package-lock.json    committed
sdk/web/LICENSE              MIT, "Copyright (c) 2026 FeedBaxter contributors"
sdk/web/README.md            the npm README (section "README")
sdk/web/tsconfig.json        typecheck config (src + tests + vite.config.ts), noEmit
sdk/web/tsconfig.build.json  declaration emit only: src minus *.test.ts, outDir dist
sdk/web/vite.config.ts       library build, two modes (ESM and IIFE), vitest config
sdk/web/scripts/check.mjs    post-build checks: size budget, IIFE smoke, source rules (Node stdlib only)
sdk/web/src/index.ts         ESM entry: re-exports createClient and the types, nothing else
sdk/web/src/client.ts        createClient, send, flush, pending, retry loop
sdk/web/src/backlog.ts       guarded storage read/write, caps
sdk/web/src/id.ts            idempotency key generation
sdk/web/src/global.ts        IIFE entry: createClient, init, send
sdk/web/src/client.test.ts   jsdom tests
sdk/web/src/node.test.ts     `// @vitest-environment node` tests
sdk/web/src/global.test.ts   init/send wrapper tests (source, jsdom)
.github/workflows/ci.yml     new `sdk-web` job
Makefile                     new `sdk-web` target, .PHONY updated
README.md                    Documentation link + one line under License
docs/ARCHITECTURE.md         tree line for sdk/web/
docs/BACKLOG.md              deferred rows (section "Docs")
```

The file split is a suggestion; the exported surface, behaviour and test names are not. Keep the source
small: the IIFE budget is 4 KiB gzipped.

## Package and toolchain

`sdk/web/package.json`:

```json
{
  "name": "@feedbaxter/web",
  "version": "0.1.0",
  "description": "Send app feedback to FeedBaxter from the browser.",
  "license": "MIT",
  "type": "module",
  "sideEffects": false,
  "files": ["dist", "README.md"],
  "main": "./dist/index.js",
  "module": "./dist/index.js",
  "types": "./dist/index.d.ts",
  "exports": {
    ".": { "types": "./dist/index.d.ts", "import": "./dist/index.js" },
    "./feedbaxter.js": "./dist/feedbaxter.js",
    "./package.json": "./package.json"
  },
  "unpkg": "./dist/feedbaxter.js",
  "jsdelivr": "./dist/feedbaxter.js",
  "repository": { "type": "git", "url": "git+https://github.com/njkuhne/FeedBaxter.git", "directory": "sdk/web" },
  "engines": { "node": ">=22" },
  "scripts": {
    "typecheck": "tsc --noEmit -p tsconfig.json",
    "test": "vitest run && npm run build",
    "build": "vite build --mode esm && vite build --mode iife && tsc -p tsconfig.build.json && node scripts/check.mjs",
    "size": "node scripts/check.mjs"
  },
  "devDependencies": { "jsdom": "29.1.1", "typescript": "7.0.2", "vite": "8.3.5", "vitest": "4.1.12" }
}
```

- No `private` field, no `dependencies`, no `prepublish`/`publish` scripts. Nothing runs `npm publish`.
- The package is MIT (its own `LICENSE`); the repo stays AGPL. `README.md` (root) gets one line under
  License: "The Web SDK in `sdk/web` is MIT-licensed so it can be embedded in any app; see
  `sdk/web/LICENSE`." `npm pack` includes `LICENSE` automatically.
- `npm test` builds, so the size budget and IIFE smoke run in `npm test` even though CI runs `test`
  before `build`. Building twice in CI is cheap.
- `vite.config.ts`: `build.lib` with `formats` chosen by `mode`:
  - `esm`: entry `src/index.ts`, format `es`, file `index.js`, `minify: false`, `emptyOutDir: true`.
  - `iife`: entry `src/global.ts`, format `iife`, `name: "FeedBaxter"`, file `feedbaxter.js`,
    `minify: true`, `emptyOutDir: false`.
  - Both: `target: "es2020"`, `sourcemap: false`, `outDir: "dist"`. Test block: `environment: "jsdom"`,
    `globals: false`, `include: ["src/**/*.test.ts"]`. Same `declare const process` trick as
    `web/vite.config.ts` if anything Node-ish is needed; do not add `@types/node`.
- `tsconfig.json`: like `web/tsconfig.json` minus JSX/React (`target ES2020`, `lib ["ES2020","DOM"]`,
  strict, `noUncheckedIndexedAccess`, `noUnused*`, `isolatedModules`, `skipLibCheck`, `types: []`).
  `tsconfig.build.json` extends it with `noEmit: false`, `declaration: true`, `emitDeclarationOnly: true`,
  `outDir: "dist"`, `rootDir: "src"`, `include ["src"]`, `exclude ["src/**/*.test.ts"]`. Result:
  `dist/index.d.ts` (+ the module .d.ts files it re-exports from). If TypeScript 7.0.2 cannot emit
  declarations, stop and report; do not add a plugin.
- `scripts/check.mjs` (Node stdlib only; `node:zlib` gzipSync level 9), fails with a non-zero exit on any of:
  1. `dist/index.js`, `dist/index.d.ts` or `dist/feedbaxter.js` missing.
  2. `gzip(dist/feedbaxter.js)` > 4096 bytes. Always print `feedbaxter.js: N bytes, N gzipped (budget 4096)`.
  3. IIFE smoke: run `dist/feedbaxter.js` in `node:vm` with a context `{ window: ctx }` style global
     (`vm.createContext({})`, then `ctx.window = ctx`): `FeedBaxter.createClient`, `.init`, `.send` are
     functions, and `FeedBaxter.send({message:"x"})` throws an Error whose message mentions `init`.
  4. Source rules over `src/*.ts` excluding tests: no `document.`, `navigator.`, `location.`, `cookie`,
     `innerHTML`, `console.`, no `: any`/`as any`, no `http://`/`https://`.
  5. `package.json` has no `dependencies` key (or it is empty) and no `private` key.

## Exported surface (ESM, `src/index.ts`)

Exactly this (Fable's fixed API):

```ts
export interface Options {
  url: string;                 // FeedBaxter origin, e.g. https://feedback.example.com; trailing slashes stripped
  key: string;                 // ingest key (publishable)
  appVersion?: string;
  platform?: string;           // default "web"
  storage?: Storage | null;    // default window.localStorage when present and accessible; null = no backlog
  fetch?: typeof fetch;        // default globalThis.fetch, looked up in createClient, not at import
  maxAttempts?: number;        // in-session fetch attempts per send, default 4
  backlogLimit?: number;       // items, default 20
  backlogBytes?: number;       // UTF-8 bytes of the serialized backlog, default 32768
}
export interface SendInput {
  message: string;
  userId?: string;
  metadata?: Record<string, unknown>;
  key?: string;                // overrides Options.key for this submission
  idempotencyKey?: string;     // caller-supplied; otherwise generated
}
export type SendResult = { ok: true } | { ok: false; error: string; retryable: boolean; queued: boolean };
export interface Client { send(input: SendInput): Promise<SendResult>; flush(): Promise<void>; pending(): number; }
export function createClient(opts: Options): Client;
```

`createClient` throws a `TypeError` (synchronously, the only throw in the ESM API) when `url` or `key` is
not a non-empty string, or when no `fetch` is available. `maxAttempts` < 1 is treated as 1;
`backlogLimit`/`backlogBytes` ≤ 0 mean "no backlog".

**`send` never rejects** and never throws. Every path resolves a `SendResult`; an unexpected exception
inside resolves `{ok:false, error:"internal", retryable:false, queued:false}`.

IIFE (`src/global.ts`, exposed as `window.FeedBaxter` through the IIFE `name`):

```ts
export { createClient };
export function init(opts: Options): Client;          // creates (or replaces) the default client, returns it
export function send(input: SendInput): Promise<SendResult>; // default client's send
```

`send` before `init` throws synchronously: `new Error("FeedBaxter: call FeedBaxter.init({ url, key }) before send().")`.
No auto-init from `data-` attributes or script URLs.

Importing either entry touches no globals (no `window`, `localStorage`, `crypto` or `fetch` access at
module scope; `sideEffects: false` must be true).

## Error codes in `SendResult.error`

| `error` | Source | `retryable` | Notes |
| --- | --- | --- | --- |
| `invalid_payload` | local check, or server 400 | false | see Local validation |
| `payload_too_large` | local check, or server 413 | false | |
| `invalid_key` | server 401 | false | |
| `rate_limited` | 429 | true | |
| `daily_limit` | 429 | true | |
| `plan_limit` | 429 | false | stops the session |
| `queue_full` | 503 | true | |
| `network_error` | fetch rejected or timed out | true | |
| `server_error` | other 5xx, or 408 | true | |
| `http_error` | any other non-202 status | false | e.g. 404 from a wrong `url` |
| `internal` | unexpected exception in the SDK | false | |

For a server response, `error` is the body's `error` string when the body is JSON with a string `error`
field and the status is one the table maps (400, 401, 413, 429, 503); otherwise the status class decides
(`server_error` / `http_error`). A 429 with an unknown or missing code is treated as `rate_limited`.
Response bodies are read with `res.json()` inside try/catch and only for non-202 statuses.

## Behaviour

### Request

`POST ${url}/v1/feedback`, `fetch` options: `method: "POST"`, `headers: { "Content-Type":
"application/json", "Authorization": "Bearer " + (input.key || opts.key) }`, `body`, `credentials:
"omit"` (no cookies, ever), `signal` from an `AbortController` aborted after 15 s (constant
`REQUEST_TIMEOUT_MS`; an abort is a `network_error`). No `keepalive`, no other headers.

Body (key order as written; absent optional fields are omitted, never `null`):
`{ message, userId?, idempotencyKey, appVersion?, platform, metadata? }`. `platform` = `opts.platform ??
"web"`. `userId` and `metadata` are passed through exactly as given; the SDK never adds fields.

### Idempotency key

Generated once per `send` (unless `input.idempotencyKey` is given) and stored in the body, so it is reused
on every in-session retry and on every backlog flush after a reload. Generation (`id.ts`):
`globalThis.crypto.randomUUID()` when it exists; else a v4 UUID built from
`crypto.getRandomValues(new Uint8Array(16))`; else (no Web Crypto at all) a v4-shaped string from
`Math.random()` mixed with `Date.now()`. The last fallback is documented in the README as weaker
(collisions only matter across one tenant's own keys; the server dedups per tenant).

### Local validation (before any network or storage access, never retried, never queued)

- `message` not a string, or `message.trim() === ""` → `invalid_payload`.
- `message` longer than 5000 **code points** (count with `for…of` or `Array.from`, matching Go's rune
  count, not `.length`) → `payload_too_large`. Never truncate.
- `metadata` present and not a plain object (null, array, non-object) → `invalid_payload`;
  `JSON.stringify(metadata)` throws (cycle, BigInt) → `invalid_payload`; its UTF-8 byte length > 4096 →
  `payload_too_large`.
- `userId` present and not a string → `invalid_payload`.
- `idempotencyKey` given and (not a string, empty, > 128 code points, or contains a C0/C1 control
  character) → `invalid_payload`.
- Serialized body UTF-8 bytes (TextEncoder) > 16384 → `payload_too_large`.

These are the server's default `MAX_*` limits as constants (`MAX_MESSAGE_CHARS`, `MAX_METADATA_BYTES`,
`MAX_BODY_BYTES`). A server configured lower still answers 400/413, which the SDK handles as above.

### `send` sequence

1. Local validation (above). Fail → resolve not-retryable, not queued.
2. Build the body with its idempotency key.
3. Kick off `flush()` without awaiting it (it is single-flight and skips this item; see Backlog).
4. If the client is `planLimited` → store in the backlog, resolve `{ok:false, error:"plan_limit",
   retryable:false, queued:<stored>}`. No fetch.
5. If the client is paused (`pausedUntil > Date.now()`, see 429 below) → store in the backlog with
   `notBefore = pausedUntil`, resolve `{ok:false, error:<pause reason>, retryable:true, queued:<stored>}`. No fetch.
6. Attempt loop, `attempt = 1..maxAttempts`. Before each attempt re-check `planLimited` (another send may
   have hit it) and, if set, go to step 4's outcome. One fetch per attempt:
   - **202** → remove from backlog if it was persisted; resolve `{ok:true}`.
   - **Not retryable** (400, 401, 413, `http_error`) → remove from backlog if persisted; resolve
     `{ok:false, error, retryable:false, queued:false}`.
   - **`plan_limit`** → set `planLimited = true`; store (or keep) in backlog; resolve
     `{ok:false, error:"plan_limit", retryable:false, queued:<stored>}`.
   - **Retryable** (`rate_limited`, `daily_limit`, `queue_full`, `server_error`, `network_error`):
     a. On the first retryable failure, persist the item to the backlog **before waiting** (write-ahead,
        so a reload during the wait loses nothing) and mark its idempotency key in the client's in-memory
        `inFlight` set so `flush` skips it. The happy path writes nothing to storage.
     b. Delay: `backoff = min(30000, 1000 * 2^(attempt-1))`, jittered as `backoff/2 + random*backoff/2`.
        If the response carried a valid `Retry-After` (non-negative integer seconds; HTTP-date form is
        ignored), `delay = max(jittered, retryAfter*1000)`.
     c. If `delay > 60000` (`MAX_IN_SESSION_WAIT_MS`): set the item's `notBefore = Date.now() + delay` in
        the backlog; for `rate_limited`/`daily_limit` also set the client's `pausedUntil` to that time
        and the pause reason to the code; stop, resolve `{ok:false, error, retryable:true, queued:<stored>}`.
     d. If this was the last attempt: stop, resolve `{ok:false, error, retryable:true, queued:<stored>}`
        (item stays in the backlog with no `notBefore`, so the next trigger retries it).
     e. Otherwise wait `delay` and loop.
   - Always remove the key from `inFlight` when the loop ends.
7. `queued` is true only if the item is actually in storage when `send` resolves (storage present, write
   succeeded, not evicted by the caps). With no storage, retryable failures resolve `queued:false`.

Worst case a `send` takes `maxAttempts` fetches (each ≤ 15 s) plus waits of at most 60 s each, so its
promise always settles within minutes; long waits (daily limit, plan limit, offline) resolve immediately
with `queued:true` and the backlog carries the item.

`planLimited` and `pausedUntil` are per client instance, in memory only: a reload or a new `createClient`
starts clean and the backlog flush probes the server again.

### Backlog (`backlog.ts`)

- One storage key, `feedbaxter.pending.v1`. Value: JSON array, oldest first, of
  `{ idempotencyKey: string, body: <the request body object>, key?: string, notBefore?: number }`.
  `key` is stored only when the send used a per-call key; items without it use the client's key at flush
  time (so a rotated key in a new release applies to old items).
- Every access (`getItem`, `setItem`, `removeItem`, `JSON.parse`, and reading `window.localStorage`
  itself) is in try/catch. Unparseable or non-array value → treated as empty (and overwritten on the next
  write). Any failure means "no backlog" for that operation; it never throws out of `send`, `flush` or
  `pending`.
- Writes are read-modify-write of the whole array, synchronous, keyed by `idempotencyKey` (an existing
  entry with the same key is replaced in place, never duplicated). After each add, enforce the caps:
  drop oldest until `length ≤ backlogLimit` and the UTF-8 byte length of the serialized array ≤
  `backlogBytes`. An item that alone exceeds `backlogBytes` is not stored. Empty array → `removeItem`.
- `pending()` = current backlog length (0 without storage or on any failure). Items being retried
  in-session are already in the backlog after their first failure, so they count.

### `flush()`

Single-flight: while a flush runs, `flush()` returns the same promise. Resolves `void`, never rejects.

1. Skip entirely if `planLimited` or `pausedUntil > now`.
2. Snapshot the backlog. For each item oldest first, skipping keys in `inFlight` and items with
   `notBefore > now`: one fetch (no in-session backoff here), with `item.key ?? opts.key`.
   - 202 or not retryable → remove the item (fresh read-modify-write by `idempotencyKey`).
   - `plan_limit` → set `planLimited`, stop the flush.
   - `rate_limited`/`daily_limit` → set the item's `notBefore` from `Retry-After` when present
     (`now + seconds*1000`); `daily_limit` with `Retry-After` also pauses the client; stop the flush.
   - `queue_full`, `server_error`, `network_error` → leave the item, stop the flush (the server or
     network is down; do not walk the rest of the queue).

Triggers: (a) `createClient` schedules one with `setTimeout(…, 0)` (never synchronously in the
constructor); (b) when `typeof window !== "undefined"` and `window.addEventListener` exists,
`createClient` adds an `online` listener that calls `flush()`; (c) every `send` (step 3). No other
background timers.

Several clients on one page (or several tabs) share the storage key. That is safe: the server dedups by
idempotency key, so a concurrent double flush costs a duplicate request, not a duplicate row.

### Privacy

The SDK reads only what it is given and `localStorage`. It never reads cookies (and sends with
`credentials: "omit"`), `navigator`, `location`, `document`, `referrer`, screen or time-zone data, and
sends nothing anywhere but `${url}/v1/feedback`. No logging (`console.*` is banned by check.mjs). `userId`
is passed through as given; the README says it must be an opaque ID, never an email.

### Node / SSR

`createClient` works in Node 22+ with the global `fetch`: `window` is undefined, so storage defaults to
none and no `online` listener is added. Retries still work in memory.

## Tests

Vitest + jsdom, mocked `fetch` (a `vi.fn` returning `new Response(...)`, or rejecting for network
errors), `vi.useFakeTimers()` in every test that waits (advance with `vi.advanceTimersByTimeAsync`), and
`vi.spyOn(Math, "random")` where jitter matters. The whole suite must run in a few seconds. Each test
proves one thing. Use a fresh in-memory `Storage` stub (a small class implementing `getItem`/`setItem`/
`removeItem`/`clear`/`key`/`length`) or jsdom's `localStorage` cleared in `beforeEach`.

`src/client.test.ts`:

| Test | Proves |
| --- | --- |
| `accepted returns ok` | 202 → `{ok:true}`; one fetch to `https://fb.test/v1/feedback` (trailing slash on `url` stripped); method POST; headers exactly Content-Type + `Authorization: Bearer k1`; `credentials: "omit"`; body has `message`, `userId`, `metadata`, `appVersion`, `platform:"web"`, a UUID-shaped `idempotencyKey`, and no other keys; nothing written to storage. |
| `optional fields omitted` | Without userId/metadata/appVersion the body has exactly `message`, `idempotencyKey`, `platform`; `platform` option overrides `"web"`. |
| `idempotency key stable across retries` | 503, 503, 202 → three fetches with the same `idempotencyKey`; `{ok:true}`; backlog empty afterwards. |
| `idempotency key survives backlog reload` | maxAttempts 1 + 503 → `queued:true`; a second client on the same storage with fetch → 202 flushes it with the same key; backlog empty. |
| `caller idempotency key used` | `input.idempotencyKey` is sent verbatim; a 129-code-point one → `invalid_payload`, no fetch. |
| `invalid key not retried` | 401 `invalid_key` → `{ok:false,error:"invalid_key",retryable:false,queued:false}`, exactly one fetch, nothing stored. |
| `payload too large not retried` | 413 → `payload_too_large`, one fetch, not queued. Same for 400 `invalid_payload` and a 404 (`http_error`). |
| `local validation` (table) | empty / whitespace message, non-string message, 5001-code-point message (built from an astral emoji so `.length` would differ), metadata array, metadata > 4096 bytes, cyclic metadata, non-string userId, body > 16384 bytes → right code, `retryable:false`, `queued:false`, zero fetches, nothing stored. (A 5000-emoji message is 20000 bytes, so it fails the body limit; code-point counting is proven by `local validation counts code points not UTF-16 units`: 2600 emoji and 5000 × "é" are accepted.) |
| `rate limited honours Retry-After` | 429 `rate_limited` with `Retry-After: 7`, then 202: no second fetch at 6.9 s, second fetch by 7 s; `{ok:true}`. |
| `long Retry-After goes to the backlog` | 429 `daily_limit` with `Retry-After: 3600` → resolves immediately (no timer advance needed beyond microtasks) `{ok:false,error:"daily_limit",retryable:true,queued:true}`; stored item has `notBefore ≈ now+3600 s`; a following `send` makes no fetch and is queued (client paused). |
| `queue full backs off then queues` | 503 `queue_full` every time, maxAttempts 4 → 4 fetches with growing gaps (assert no fetch before each minimum delay with `Math.random` = 0, i.e. ≥ 0.5 s, 1 s, 2 s — but honouring `Retry-After: 5` from the server as a floor, so gaps ≥ 5 s), then `{ok:false,error:"queue_full",retryable:true,queued:true}`; `pending()` 1. |
| `network error backs off then queues` | fetch rejects every time → 4 attempts, `network_error`, queued; item persisted after the first failure (assert storage holds it before the timers run out). |
| `request timeout is a network error` | fetch never resolves; after 15 s the abort fires → `network_error`, retried. |
| `backlog flushes on new client` | Two queued items in storage; `createClient` makes no fetch synchronously; after `setTimeout(0)` both are sent oldest first; storage key removed. |
| `backlog flushes on online` | Item queued; dispatching `window` `online` sends it. |
| `flush is single-flight` | Two concurrent `flush()` calls return the same promise and each backlog item is fetched once. |
| `flush stops on server failure` | Three items, first gets 503 → one fetch only, all three remain. |
| `backlog caps by items` | backlogLimit 3, five queued sends → `pending()` 3, the three newest kept. |
| `backlog caps by bytes` | backlogBytes ~ 2 items' worth → oldest dropped; an item larger than the cap alone resolves `queued:false`. |
| `plan limit stops the session` | 429 `plan_limit` → one fetch, `{ok:false,error:"plan_limit",retryable:false,queued:true}`; a later `send` makes no fetch and is queued with `plan_limit`; `flush()` makes no fetch; a new client on the same storage probes again (one fetch). |
| `storage that throws is tolerated` | Storage whose every method throws (and a `storage` getter that throws via jsdom `window.localStorage` stubbed with a throwing getter): `send` resolves normally for 202, 401 and exhausted 503 (`queued:false`); `pending()` 0; `flush()` resolves; corrupt JSON in storage is treated as empty. |
| `no storage means no backlog` | `storage: null`, exhausted 503 → `queued:false`, `retryable:true`. |
| `per-call key overrides` | `send({key:"k2"})` uses `Bearer k2`; when queued, the stored item carries `key:"k2"` and the flush uses it; items without `key` flush with the client key. |
| `send never rejects` | fetch that throws synchronously, and a `Response` whose `json()` throws, both resolve a `SendResult`. |
| `createClient validates options` | missing url, empty key → TypeError. |

`src/node.test.ts` (`// @vitest-environment node`):

| Test | Proves |
| --- | --- |
| `import touches no globals` | With `globalThis.fetch`, `crypto`, `localStorage` replaced by throwing getters (restore after), a dynamic `import("./index")` succeeds. |
| `works without window` | `typeof window === "undefined"`; `createClient` with a mocked fetch → 202 `{ok:true}`; exhausted 503 → `queued:false`; `pending()` 0. |
| `idempotency fallback without randomUUID` | With `crypto.randomUUID` absent, keys are still v4-shaped and distinct over 100 calls. |

`src/global.test.ts`:

| Test | Proves |
| --- | --- |
| `send before init throws` | Error message mentions `init`. |
| `init then send` | `init({...fetch})` returns a client; `send` posts through it; a second `init` replaces the default. |

`scripts/check.mjs` (runs in `npm test` via `npm run build`) covers the size budget, the IIFE smoke and
the source rules.

## CI, Makefile, docs

- `.github/workflows/ci.yml`, new job after `ui`:
  ```yaml
  sdk-web:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: npm
          cache-dependency-path: sdk/web/package-lock.json
      - run: cd sdk/web && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run build
  ```
- `Makefile`: `sdk-web: ## Build and test the web SDK (needs Node 22+)` →
  `cd sdk/web && npm ci --no-audit --no-fund && npm run typecheck && npm test && npm run build`; add
  `sdk-web` to `.PHONY`.
- `sdk/web/README.md`: install (`npm install @feedbaxter/web`); ESM example with `createClient` and
  `send`; script tag example (`<script src="https://feedback.example.com/…/feedbaxter.js">` is **not**
  served by the binary in v1, so show self-hosting the file from `node_modules/@feedbaxter/web/dist/` or a
  CDN path for the package, written as a placeholder); options table with defaults; `SendResult` and the
  error code table with what the SDK does for each; retry and backoff; backlog semantics (where it lives,
  caps, when it flushes, `plan_limit` stop, `queued` meaning, no storage in Node); idempotency; limits
  (5000 characters, 4096-byte metadata, 16384-byte body, checked locally); privacy (no cookies, no
  fingerprinting, no telemetry, `userId` opaque and never an email, the publishable key is safe to ship
  but only creates feedback); license MIT. Plain voice, sentence case, no marketing, no logo.
- Root `README.md`: Documentation list gets `- [Web SDK](sdk/web/README.md)` after the MCP link; the
  License line for `sdk/web`.
- `docs/ARCHITECTURE.md` §2 tree: `sdk/web/               npm package (@feedbaxter/web, MIT) and script tag build; see sdk/web/README.md`.
- `docs/SPEC.md`: unchanged (its SDK section has no doc-pointer pattern).
- `docs/BACKLOG.md` rows (Raised in `M5`):
  - Serve `feedbaxter.js` from the binary (for example `/sdk/feedbaxter.js`) so self-hosters need no CDN.
  - No send on page unload (`keepalive` fetch or `sendBeacon`); a submission made just before the tab
    closes can be lost if its first attempt has not finished.
  - No periodic backlog timer: queued items retry on the next `send`, `online` event or page load only.
  - Several tabs share one backlog without locking; concurrent flushes can send an item twice (deduped by
    the server) and in a rare write race one tab's addition can be lost.
  - npm publishing (scope `@feedbaxter`, provenance, release workflow) is not set up.
  - Swift SDK is M5 part 2.

## Acceptance

`cd sdk/web && npm ci && npm run typecheck && npm test && npm run build` green on Node 22+ (25 locally);
`check.mjs` prints the gzipped size ≤ 4096; zero `dependencies`; `git status` shows only files under
`sdk/web/` plus `.github/workflows/ci.yml`, `Makefile`, `README.md`, `docs/ARCHITECTURE.md`,
`docs/BACKLOG.md` and this brief. No Go file and nothing under `web/` changed.

## Decisions (made by Opus beyond Fable's fixed list)

1. `send` resolves `{ok:false, queued:true}` as soon as in-session attempts are exhausted or a wait
   would exceed 60 s; it never waits for the backlog.
2. Write-ahead: an item is persisted on its **first** retryable failure, before the backoff wait, so a
   reload mid-retry loses nothing; the happy path never touches storage. An in-memory `inFlight` set stops
   the client's own flush from double-sending it.
3. Each fetch has a 15 s timeout (abort → `network_error`) so a hung connection cannot hang `send`. Not an
   option in v1.
4. `Retry-After` is honoured as a floor on every retryable response, including the server's
   `Retry-After: 5` on 503, not only on 429.
5. A long `daily_limit`/`rate_limited` wait pauses the whole client until `notBefore`; sends during the
   pause go straight to the backlog. The daily cap is tenant-wide, so hammering would only collect 429s.
6. Error codes beyond the server's: `network_error`, `server_error` (5xx/408), `http_error` (other
   non-202, not retryable), `internal`; local checks reuse `invalid_payload` and `payload_too_large`.
7. Local message length counts code points to match the server's rune count.
8. `credentials: "omit"` on every request.
9. ESM only (no CommonJS build); Node 22 can `require()` ESM.
10. `queued` is true only if the item is really in storage when `send` resolves.
11. The IIFE has no auto-init; `send` before `init` throws synchronously (the one throwing path).
12. The package is MIT inside an AGPL repo (Fable's decision 1; flagged to the owner).
13. Relative imports in `src/` carry a `.js` extension so the emitted `.d.ts` files resolve for consumers
    using `moduleResolution: node16`/`nodenext` (checked with a scratch consumer project).
14. The server's error body is read only for 429 (to tell `rate_limited`, `daily_limit` and `plan_limit`
    apart); 400, 401, 413 and 503 each map to their one fixed code.
