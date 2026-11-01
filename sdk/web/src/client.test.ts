import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createClient, type Client, type Options, type SendInput } from "./client";

const KEY = "feedbaxter.pending.v1";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

class MemStorage implements Storage {
  data = new Map<string, string>();
  writes = 0;
  get length(): number {
    return this.data.size;
  }
  clear(): void {
    this.data.clear();
  }
  getItem(k: string): string | null {
    return this.data.get(k) ?? null;
  }
  key(i: number): string | null {
    return [...this.data.keys()][i] ?? null;
  }
  removeItem(k: string): void {
    this.writes++;
    this.data.delete(k);
  }
  setItem(k: string, v: string): void {
    this.writes++;
    this.data.set(k, v);
  }
  items(): { idempotencyKey: string; body: { message: string }; key?: string; notBefore?: number }[] {
    const raw = this.data.get(KEY);
    return raw ? JSON.parse(raw) : [];
  }
}

class ThrowingStorage implements Storage {
  get length(): number {
    throw new Error("blocked");
  }
  clear(): void {
    throw new Error("blocked");
  }
  getItem(): string | null {
    throw new Error("blocked");
  }
  key(): string | null {
    throw new Error("blocked");
  }
  removeItem(): void {
    throw new Error("blocked");
  }
  setItem(): void {
    throw new Error("blocked");
  }
}

const res = (status: number, body?: unknown, headers?: Record<string, string>): Response =>
  new Response(body === undefined ? null : JSON.stringify(body), { status, headers });
const accepted = (): Response => res(202);

type Mock = ReturnType<typeof vi.fn<typeof fetch>>;
const bodyOf = (f: Mock, n = 0): Record<string, unknown> => JSON.parse(String(f.mock.calls[n]?.[1]?.body));
const headersOf = (f: Mock, n = 0): Record<string, string> => f.mock.calls[n]?.[1]?.headers as Record<string, string>;
const seed = (s: MemStorage, msgs: string[]): void => {
  s.setItem(
    KEY,
    JSON.stringify(msgs.map((m) => ({ idempotencyKey: "id-" + m, body: { message: m, idempotencyKey: "id-" + m, platform: "web" } }))),
  );
  s.writes = 0;
};

let storage: MemStorage;
beforeEach(() => {
  vi.useFakeTimers();
  storage = new MemStorage();
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function mk(f: Mock, extra: Partial<Options> = {}, drain = true): Promise<Client> {
  const c = createClient({ url: "https://fb.test", key: "k1", fetch: f, storage, ...extra });
  if (drain) await vi.advanceTimersByTimeAsync(0);
  return c;
}
const fetchMock = (impl?: () => Promise<Response>): Mock => vi.fn<typeof fetch>(impl ?? (async () => accepted()));
const always = (status: number, body?: unknown, headers?: Record<string, string>): Mock =>
  fetchMock(async () => res(status, body, headers));

describe("send", () => {
  it("accepted returns ok", async () => {
    const f = fetchMock();
    const c = await mk(f, { url: "https://fb.test///", appVersion: "1.2.3" });
    const r = await c.send({ message: "hi", userId: "u1", metadata: { a: 1 } });
    expect(r).toEqual({ ok: true });
    expect(f).toHaveBeenCalledTimes(1);
    const [url, init] = f.mock.calls[0]!;
    expect(url).toBe("https://fb.test/v1/feedback");
    expect(init?.method).toBe("POST");
    expect(init?.credentials).toBe("omit");
    expect(headersOf(f)).toEqual({ "Content-Type": "application/json", Authorization: "Bearer k1" });
    const b = bodyOf(f);
    expect(Object.keys(b).sort()).toEqual(["appVersion", "idempotencyKey", "message", "metadata", "platform", "userId"]);
    expect(b).toMatchObject({ message: "hi", userId: "u1", metadata: { a: 1 }, appVersion: "1.2.3", platform: "web" });
    expect(b.idempotencyKey).toMatch(UUID);
    expect(storage.writes).toBe(0);
  });

  it("optional fields omitted", async () => {
    const f = fetchMock();
    const c = await mk(f);
    await c.send({ message: "hi" });
    expect(Object.keys(bodyOf(f)).sort()).toEqual(["idempotencyKey", "message", "platform"]);
    const f2 = fetchMock();
    const c2 = await mk(f2, { platform: "kiosk" });
    await c2.send({ message: "hi" });
    expect(bodyOf(f2).platform).toBe("kiosk");
  });

  it("idempotency key stable across retries", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const f = fetchMock();
    f.mockResolvedValueOnce(res(503, { error: "queue_full" })).mockResolvedValueOnce(res(503, { error: "queue_full" }));
    const c = await mk(f);
    const p = c.send({ message: "hi" });
    await vi.advanceTimersByTimeAsync(10000);
    expect(await p).toEqual({ ok: true });
    expect(f).toHaveBeenCalledTimes(3);
    const ids = [0, 1, 2].map((i) => bodyOf(f, i).idempotencyKey);
    expect(new Set(ids).size).toBe(1);
    expect(storage.getItem(KEY)).toBeNull();
    expect(c.pending()).toBe(0);
  });

  it("idempotency key survives backlog reload", async () => {
    const f1 = always(503, { error: "queue_full" });
    const c1 = await mk(f1, { maxAttempts: 1 });
    const r = await c1.send({ message: "hi" });
    expect(r).toEqual({ ok: false, error: "queue_full", retryable: true, queued: true });
    const id = bodyOf(f1).idempotencyKey;
    const f2 = fetchMock();
    const c2 = await mk(f2);
    await c2.flush();
    expect(f2).toHaveBeenCalledTimes(1);
    expect(bodyOf(f2).idempotencyKey).toBe(id);
    expect(storage.getItem(KEY)).toBeNull();
    expect(c2.pending()).toBe(0);
  });

  it("caller idempotency key used", async () => {
    const f = fetchMock();
    const c = await mk(f);
    expect(await c.send({ message: "hi", idempotencyKey: "my-key-1" })).toEqual({ ok: true });
    expect(bodyOf(f).idempotencyKey).toBe("my-key-1");
    const f2 = fetchMock();
    const c2 = await mk(f2);
    const r = await c2.send({ message: "hi", idempotencyKey: "😀".repeat(129) });
    expect(r).toEqual({ ok: false, error: "invalid_payload", retryable: false, queued: false });
    expect(f2).not.toHaveBeenCalled();
  });

  it("invalid key not retried", async () => {
    const f = always(401, { error: "invalid_key" });
    const c = await mk(f);
    const r = await c.send({ message: "hi" });
    expect(r).toEqual({ ok: false, error: "invalid_key", retryable: false, queued: false });
    expect(f).toHaveBeenCalledTimes(1);
    expect(storage.getItem(KEY)).toBeNull();
  });

  it.each([
    [413, { error: "payload_too_large" }, "payload_too_large"],
    [400, { error: "invalid_payload" }, "invalid_payload"],
    [404, undefined, "http_error"],
  ])("payload too large not retried (%i)", async (status, body, code) => {
    const f = always(status, body);
    const c = await mk(f);
    const r = await c.send({ message: "hi" });
    expect(r).toEqual({ ok: false, error: code, retryable: false, queued: false });
    expect(f).toHaveBeenCalledTimes(1);
    expect(storage.getItem(KEY)).toBeNull();
  });

  const bad = (input: unknown): SendInput => input as SendInput;
  const cyclic: Record<string, unknown> = {};
  cyclic.self = cyclic;
  it.each([
    ["empty message", { message: "" }, "invalid_payload"],
    ["whitespace message", { message: " \n\t " }, "invalid_payload"],
    ["non-string message", { message: 42 }, "invalid_payload"],
    ["5001 code points", { message: "😀".repeat(5001) }, "payload_too_large"],
    ["array metadata", { message: "a", metadata: [1] }, "invalid_payload"],
    ["null metadata", { message: "a", metadata: null }, "invalid_payload"],
    ["metadata over 4096 bytes", { message: "a", metadata: { x: "y".repeat(4100) } }, "payload_too_large"],
    ["cyclic metadata", { message: "a", metadata: cyclic }, "invalid_payload"],
    ["non-string userId", { message: "a", userId: 5 }, "invalid_payload"],
    ["control char in idempotency key", { message: "a", idempotencyKey: "a\nb" }, "invalid_payload"],
    ["empty idempotency key", { message: "a", idempotencyKey: "" }, "invalid_payload"],
    ["body over 16384 bytes", { message: "😀".repeat(5000) }, "payload_too_large"],
  ])("local validation: %s", async (_name, input, code) => {
    const f = fetchMock();
    const c = await mk(f);
    expect(await c.send(bad(input))).toEqual({ ok: false, error: code, retryable: false, queued: false });
    expect(f).not.toHaveBeenCalled();
    expect(storage.writes).toBe(0);
  });

  it("local validation counts code points not UTF-16 units", async () => {
    const f = fetchMock();
    const c = await mk(f);
    // 2600 emoji: .length is 5200 but 2600 code points and 10400 bytes.
    expect(await c.send({ message: "😀".repeat(2600) })).toEqual({ ok: true });
    expect(await c.send({ message: "é".repeat(5000) })).toEqual({ ok: true });
  });

  it("rate limited honours Retry-After", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const f = fetchMock();
    f.mockResolvedValueOnce(res(429, { error: "rate_limited" }, { "Retry-After": "7" }));
    const c = await mk(f);
    const p = c.send({ message: "hi" });
    await vi.advanceTimersByTimeAsync(6900);
    expect(f).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(100);
    expect(f).toHaveBeenCalledTimes(2);
    expect(await p).toEqual({ ok: true });
  });

  it("long Retry-After goes to the backlog", async () => {
    const f = always(429, { error: "daily_limit" }, { "Retry-After": "3600" });
    const c = await mk(f);
    const now = Date.now();
    const r = await c.send({ message: "hi" });
    expect(r).toEqual({ ok: false, error: "daily_limit", retryable: true, queued: true });
    const nb = storage.items()[0]?.notBefore ?? 0;
    expect(nb).toBeGreaterThanOrEqual(now + 3600_000);
    expect(nb).toBeLessThan(now + 3601_000);
    const r2 = await c.send({ message: "again" });
    expect(f).toHaveBeenCalledTimes(1);
    expect(r2).toEqual({ ok: false, error: "daily_limit", retryable: true, queued: true });
    expect(c.pending()).toBe(2);
  });

  it("queue full backs off then queues", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const f = always(503, { error: "queue_full" }, { "Retry-After": "5" });
    const c = await mk(f);
    const p = c.send({ message: "hi" });
    await vi.advanceTimersByTimeAsync(0);
    expect(f).toHaveBeenCalledTimes(1);
    for (let n = 2; n <= 4; n++) {
      await vi.advanceTimersByTimeAsync(4999);
      expect(f).toHaveBeenCalledTimes(n - 1);
      await vi.advanceTimersByTimeAsync(1);
      expect(f).toHaveBeenCalledTimes(n);
    }
    expect(await p).toEqual({ ok: false, error: "queue_full", retryable: true, queued: true });
    expect(c.pending()).toBe(1);
  });

  it("network error backs off then queues", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const f = fetchMock(async () => {
      throw new TypeError("offline");
    });
    const c = await mk(f);
    const p = c.send({ message: "hi" });
    await vi.advanceTimersByTimeAsync(0);
    expect(f).toHaveBeenCalledTimes(1);
    expect(storage.items()).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(60000);
    expect(f).toHaveBeenCalledTimes(4);
    expect(await p).toEqual({ ok: false, error: "network_error", retryable: true, queued: true });
  });

  it("request timeout is a network error", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const f = fetchMock();
    f.mockImplementation(
      (_u, init) =>
        new Promise<Response>((_, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        }),
    );
    const c = await mk(f, { maxAttempts: 2 });
    let done = false;
    const p = c.send({ message: "hi" }).then((r) => {
      done = true;
      return r;
    });
    await vi.advanceTimersByTimeAsync(14999);
    expect(done).toBe(false);
    await vi.advanceTimersByTimeAsync(1);
    expect(f).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(500);
    expect(f).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(15000);
    expect(await p).toEqual({ ok: false, error: "network_error", retryable: true, queued: true });
  });

  it("plan limit stops the session", async () => {
    const f = always(429, { error: "plan_limit" });
    const c = await mk(f);
    expect(await c.send({ message: "one" })).toEqual({ ok: false, error: "plan_limit", retryable: false, queued: true });
    expect(f).toHaveBeenCalledTimes(1);
    expect(await c.send({ message: "two" })).toEqual({ ok: false, error: "plan_limit", retryable: false, queued: true });
    await c.flush();
    expect(f).toHaveBeenCalledTimes(1);
    expect(c.pending()).toBe(2);
    const f2 = always(429, { error: "plan_limit" });
    await mk(f2);
    expect(f2).toHaveBeenCalledTimes(1);
  });

  it("storage that throws is tolerated", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const t = new ThrowingStorage();
    const c1 = await mk(fetchMock(), { storage: t });
    expect(await c1.send({ message: "a" })).toEqual({ ok: true });
    const c2 = await mk(always(401, { error: "invalid_key" }), { storage: t });
    expect(await c2.send({ message: "a" })).toMatchObject({ error: "invalid_key" });
    const c3 = await mk(always(503, { error: "queue_full" }), { storage: t, maxAttempts: 2 });
    const p = c3.send({ message: "a" });
    await vi.advanceTimersByTimeAsync(5000);
    expect(await p).toEqual({ ok: false, error: "queue_full", retryable: true, queued: false });
    expect(c3.pending()).toBe(0);
    await expect(c3.flush()).resolves.toBeUndefined();

    const orig = Object.getOwnPropertyDescriptor(window, "localStorage");
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      get() {
        throw new Error("denied");
      },
    });
    try {
      const f = fetchMock();
      const c4 = createClient({ url: "https://fb.test", key: "k1", fetch: f });
      expect(await c4.send({ message: "a" })).toEqual({ ok: true });
      expect(c4.pending()).toBe(0);
    } finally {
      if (orig) Object.defineProperty(window, "localStorage", orig);
      else delete (window as unknown as Record<string, unknown>).localStorage;
    }

    storage.setItem(KEY, "{{not json");
    const c5 = await mk(fetchMock());
    expect(c5.pending()).toBe(0);
    expect(await c5.send({ message: "a" })).toEqual({ ok: true });
  });

  it("no storage means no backlog", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const c = await mk(always(503, { error: "queue_full" }), { storage: null, maxAttempts: 2 });
    const p = c.send({ message: "a" });
    await vi.advanceTimersByTimeAsync(5000);
    expect(await p).toEqual({ ok: false, error: "queue_full", retryable: true, queued: false });
    expect(c.pending()).toBe(0);
  });

  it("per-call key overrides", async () => {
    const f1 = always(503, { error: "queue_full" });
    const c1 = await mk(f1, { maxAttempts: 1 });
    await c1.send({ message: "a", key: "k2" });
    expect(headersOf(f1).Authorization).toBe("Bearer k2");
    await c1.send({ message: "b" });
    expect(storage.items().map((i) => i.key)).toEqual(["k2", undefined]);
    const f2 = fetchMock();
    const c2 = await mk(f2, { key: "k9" });
    await c2.flush();
    expect(f2.mock.calls.map((_, i) => headersOf(f2, i).Authorization)).toEqual(["Bearer k2", "Bearer k9"]);
    expect(storage.getItem(KEY)).toBeNull();
  });

  it("send never rejects", async () => {
    const throwing = fetchMock(() => {
      throw new Error("sync");
    });
    const c1 = await mk(throwing, { maxAttempts: 1 });
    expect(await c1.send({ message: "a" })).toMatchObject({ ok: false, error: "network_error" });
    const badJson = fetchMock(
      async () =>
        ({
          status: 429,
          headers: new Headers(),
          json: () => {
            throw new Error("bad json");
          },
        }) as unknown as Response,
    );
    const c2 = await mk(badJson, { maxAttempts: 1, storage: null });
    expect(await c2.send({ message: "a" })).toEqual({ ok: false, error: "rate_limited", retryable: true, queued: false });
    const c3 = await mk(fetchMock());
    const evil = {
      get message(): string {
        throw new Error("boom");
      },
    };
    expect(await c3.send(evil)).toEqual({ ok: false, error: "internal", retryable: false, queued: false });
  });
});

describe("backlog", () => {
  it("backlog flushes on new client", async () => {
    seed(storage, ["one", "two"]);
    const f = fetchMock();
    const c = await mk(f, {}, false);
    expect(f).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(0);
    await c.flush();
    expect(f).toHaveBeenCalledTimes(2);
    expect([0, 1].map((i) => bodyOf(f, i).message)).toEqual(["one", "two"]);
    expect(storage.getItem(KEY)).toBeNull();
  });

  it("backlog flushes on online", async () => {
    const f = fetchMock();
    const c = await mk(f);
    seed(storage, ["one"]);
    window.dispatchEvent(new Event("online"));
    await c.flush();
    expect(f).toHaveBeenCalledTimes(1);
    expect(storage.getItem(KEY)).toBeNull();
  });

  it("flush is single-flight", async () => {
    const f = fetchMock();
    const c = await mk(f);
    seed(storage, ["one", "two"]);
    const p1 = c.flush();
    const p2 = c.flush();
    expect(p1).toBe(p2);
    await p1;
    expect(f).toHaveBeenCalledTimes(2);
  });

  it("flush stops on server failure", async () => {
    const f = always(503, { error: "queue_full" });
    const c = await mk(f);
    seed(storage, ["one", "two", "three"]);
    await c.flush();
    expect(f).toHaveBeenCalledTimes(1);
    expect(c.pending()).toBe(3);
  });

  it("backlog caps by items", async () => {
    const c = await mk(always(503, { error: "queue_full" }), { maxAttempts: 1, backlogLimit: 3 });
    for (let i = 1; i <= 5; i++) await c.send({ message: "m" + i });
    expect(c.pending()).toBe(3);
    expect(storage.items().map((i) => i.body.message)).toEqual(["m3", "m4", "m5"]);
  });

  it("backlog caps by bytes", async () => {
    const probe = new MemStorage();
    const c0 = await mk(always(503, { error: "queue_full" }), { maxAttempts: 1, storage: probe });
    await c0.send({ message: "m1" });
    const one = (probe.getItem(KEY) ?? "").length;
    const c = await mk(always(503, { error: "queue_full" }), { maxAttempts: 1, backlogBytes: one * 2 + 5 });
    for (let i = 1; i <= 3; i++) await c.send({ message: "m" + i });
    expect(storage.items().map((i) => i.body.message)).toEqual(["m2", "m3"]);
    const small = new MemStorage();
    const c2 = await mk(always(503, { error: "queue_full" }), { maxAttempts: 1, storage: small, backlogBytes: 100 });
    expect(await c2.send({ message: "m1" })).toEqual({ ok: false, error: "queue_full", retryable: true, queued: false });
    expect(c2.pending()).toBe(0);
  });
});

describe("createClient", () => {
  it("createClient validates options", () => {
    const f = fetchMock();
    expect(() => createClient({ key: "k", fetch: f } as unknown as Options)).toThrow(TypeError);
    expect(() => createClient({ url: "", key: "k", fetch: f })).toThrow(TypeError);
    expect(() => createClient({ url: "https://fb.test", key: "", fetch: f })).toThrow(TypeError);
    vi.stubGlobal("fetch", undefined);
    expect(() => createClient({ url: "https://fb.test", key: "k" })).toThrow(TypeError);
  });
});
