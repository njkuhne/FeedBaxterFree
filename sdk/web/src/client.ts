import { createBacklog, type Item } from "./backlog.js";
import { newId } from "./id.js";

export interface Options {
  url: string;
  key: string;
  appVersion?: string;
  platform?: string;
  storage?: Storage | null;
  fetch?: typeof fetch;
  maxAttempts?: number;
  backlogLimit?: number;
  backlogBytes?: number;
}
export interface SendInput {
  message: string;
  userId?: string;
  metadata?: Record<string, unknown>;
  key?: string;
  idempotencyKey?: string;
}
export type SendResult = { ok: true } | { ok: false; error: string; retryable: boolean; queued: boolean };
export interface Client {
  send(input: SendInput): Promise<SendResult>;
  flush(): Promise<void>;
  pending(): number;
}

const REQUEST_TIMEOUT_MS = 15000;
const MAX_IN_SESSION_WAIT_MS = 60000;
const MAX_MESSAGE_CHARS = 5000;
const MAX_METADATA_BYTES = 4096;
const MAX_BODY_BYTES = 16384;
const MAX_IDEMPOTENCY_CHARS = 128;

/** Outcome of one request. error "" means accepted. */
interface Out {
  error: string;
  retryable: boolean;
  retryAfter?: number;
}
const ACCEPTED: Out = { error: "", retryable: false };
const RETRYABLE = new Set(["rate_limited", "daily_limit", "queue_full", "server_error", "network_error"]);
const LIMITS = new Set(["rate_limited", "daily_limit", "plan_limit"]);
const BY_STATUS: Record<number, string> = {
  400: "invalid_payload",
  401: "invalid_key",
  413: "payload_too_large",
  429: "rate_limited",
  503: "queue_full",
};

const chars = (s: string): number => Array.from(s).length;
const bytes = (s: string): number => new TextEncoder().encode(s).length;
const fail = (error: string, retryable: boolean, queued: boolean): SendResult => ({ ok: false, error, retryable, queued });

async function classify(res: Response): Promise<Out> {
  if (res.status === 202) return ACCEPTED;
  let error = BY_STATUS[res.status] ?? (res.status >= 500 || res.status === 408 ? "server_error" : "http_error");
  if (res.status === 429) {
    try {
      const b: unknown = await res.json();
      const code = (b as { error?: unknown } | null)?.error;
      if (typeof code === "string" && LIMITS.has(code)) error = code;
    } catch {
      // Unknown body: treated as rate_limited.
    }
  }
  const ra = res.headers.get("Retry-After");
  const out: Out = { error, retryable: RETRYABLE.has(error) };
  if (ra !== null && /^\d+$/.test(ra.trim())) out.retryAfter = Number(ra);
  return out;
}

/** Returns the error code for a bad input, or the request body. */
function build(input: SendInput, platform: string, appVersion: string | undefined): string | Record<string, unknown> {
  const { message, userId, metadata, idempotencyKey } = input;
  if (typeof message !== "string" || message.trim() === "") return "invalid_payload";
  if (chars(message) > MAX_MESSAGE_CHARS) return "payload_too_large";
  if (metadata !== undefined) {
    if (Object.prototype.toString.call(metadata) !== "[object Object]") return "invalid_payload";
    let json: string;
    try {
      json = JSON.stringify(metadata);
    } catch {
      return "invalid_payload";
    }
    if (bytes(json) > MAX_METADATA_BYTES) return "payload_too_large";
  }
  if (userId !== undefined && typeof userId !== "string") return "invalid_payload";
  if (
    idempotencyKey !== undefined &&
    (typeof idempotencyKey !== "string" ||
      idempotencyKey === "" ||
      chars(idempotencyKey) > MAX_IDEMPOTENCY_CHARS ||
      /[\u0000-\u001f\u007f-\u009f]/.test(idempotencyKey))
  ) {
    return "invalid_payload";
  }
  const body: Record<string, unknown> = { message };
  if (userId !== undefined) body.userId = userId;
  body.idempotencyKey = idempotencyKey ?? newId();
  if (appVersion !== undefined) body.appVersion = appVersion;
  body.platform = platform;
  if (metadata !== undefined) body.metadata = metadata;
  return bytes(JSON.stringify(body)) > MAX_BODY_BYTES ? "payload_too_large" : body;
}

export function createClient(opts: Options): Client {
  if (!opts || typeof opts.url !== "string" || opts.url === "") throw new TypeError("FeedBaxter: url is required.");
  if (typeof opts.key !== "string" || opts.key === "") throw new TypeError("FeedBaxter: key is required.");
  const doFetch = opts.fetch ?? (typeof globalThis.fetch === "function" ? globalThis.fetch : undefined);
  if (!doFetch) throw new TypeError("FeedBaxter: no fetch available; pass the fetch option.");

  const endpoint = opts.url.replace(/\/+$/, "") + "/v1/feedback";
  const platform = opts.platform ?? "web";
  const maxAttempts = Math.max(1, Math.floor(opts.maxAttempts ?? 4) || 1);
  let store: Storage | null = null;
  if (opts.storage !== undefined) {
    store = opts.storage;
  } else {
    try {
      store = typeof window !== "undefined" ? (window.localStorage ?? null) : null;
    } catch {
      store = null;
    }
  }
  const backlog = createBacklog(store, opts.backlogLimit ?? 20, opts.backlogBytes ?? 32768);

  let planLimited = false;
  let pausedUntil = 0;
  let pauseReason = "rate_limited";
  let flushing: Promise<void> | undefined;
  const inFlight = new Set<string>();

  async function post(body: unknown, key: string): Promise<Out> {
    const ctl = new AbortController();
    const timer = setTimeout(() => ctl.abort(), REQUEST_TIMEOUT_MS);
    try {
      const res = await doFetch!(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json", Authorization: "Bearer " + key },
        body: JSON.stringify(body),
        credentials: "omit",
        signal: ctl.signal,
      });
      return await classify(res);
    } catch {
      return { error: "network_error", retryable: true };
    } finally {
      clearTimeout(timer);
    }
  }

  async function runFlush(): Promise<void> {
    try {
      if (planLimited || pausedUntil > Date.now()) return;
      for (const it of backlog.list()) {
        const id = it.idempotencyKey;
        if (inFlight.has(id) || (it.notBefore !== undefined && it.notBefore > Date.now())) continue;
        const o = await post(it.body, it.key ?? opts.key);
        if (o.error === "plan_limit") {
          planLimited = true;
          return;
        }
        if (o.error === "" || !o.retryable) {
          backlog.remove(id);
          continue;
        }
        if ((o.error === "rate_limited" || o.error === "daily_limit") && o.retryAfter !== undefined) {
          const until = Date.now() + o.retryAfter * 1000;
          backlog.setNotBefore(id, until);
          if (o.error === "daily_limit") {
            pausedUntil = until;
            pauseReason = o.error;
          }
        }
        return;
      }
    } catch {
      // flush never rejects
    }
  }

  function flush(): Promise<void> {
    if (!flushing) {
      flushing = runFlush().finally(() => {
        flushing = undefined;
      });
    }
    return flushing;
  }

  async function doSend(input: SendInput): Promise<SendResult> {
    const body = build(input, platform, opts.appVersion);
    if (typeof body === "string") return fail(body, false, false);
    const id = body.idempotencyKey as string;
    const item: Item = { idempotencyKey: id, body };
    if (input.key) item.key = input.key;
    const queued = (): boolean => backlog.has(id);
    const stop = (error: string, retryable: boolean): SendResult => fail(error, retryable, queued());

    void flush();

    const planStop = (): SendResult => {
      backlog.put(item);
      return stop("plan_limit", false);
    };
    if (planLimited) return planStop();
    if (pausedUntil > Date.now()) {
      item.notBefore = pausedUntil;
      backlog.put(item);
      return stop(pauseReason, true);
    }

    let persisted = false;
    try {
      for (let attempt = 1; attempt <= maxAttempts; attempt++) {
        if (planLimited) return planStop();
        const o = await post(body, item.key ?? opts.key);
        if (o.error === "") {
          if (persisted) backlog.remove(id);
          return { ok: true };
        }
        if (o.error === "plan_limit") {
          planLimited = true;
          return planStop();
        }
        if (!o.retryable) {
          if (persisted) backlog.remove(id);
          return fail(o.error, false, false);
        }
        if (!persisted) {
          backlog.put(item);
          inFlight.add(id);
          persisted = true;
        }
        const backoff = Math.min(30000, 1000 * 2 ** (attempt - 1));
        let delay = backoff / 2 + (Math.random() * backoff) / 2;
        if (o.retryAfter !== undefined) delay = Math.max(delay, o.retryAfter * 1000);
        if (delay > MAX_IN_SESSION_WAIT_MS) {
          const until = Date.now() + delay;
          item.notBefore = until;
          backlog.put(item);
          if (o.error === "rate_limited" || o.error === "daily_limit") {
            pausedUntil = until;
            pauseReason = o.error;
          }
          return stop(o.error, true);
        }
        if (attempt === maxAttempts) return stop(o.error, true);
        await new Promise<void>((r) => setTimeout(r, delay));
      }
      return fail("internal", false, false);
    } finally {
      inFlight.delete(id);
    }
  }

  setTimeout(() => void flush(), 0);
  if (typeof window !== "undefined" && typeof window.addEventListener === "function") {
    window.addEventListener("online", () => void flush());
  }

  return {
    async send(input) {
      try {
        return await doSend(input);
      } catch {
        return fail("internal", false, false);
      }
    },
    flush,
    pending: () => backlog.list().length,
  };
}
