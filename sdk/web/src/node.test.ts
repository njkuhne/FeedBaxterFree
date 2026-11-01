// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("node", () => {
  it("import touches no globals", async () => {
    const names = ["fetch", "crypto", "localStorage"];
    const saved = names.map((n) => Object.getOwnPropertyDescriptor(globalThis, n));
    for (const n of names) {
      Object.defineProperty(globalThis, n, {
        configurable: true,
        get() {
          throw new Error("touched " + n);
        },
      });
    }
    try {
      const m = await import("./index");
      expect(typeof m.createClient).toBe("function");
    } finally {
      names.forEach((n, i) => {
        const d = saved[i];
        if (d) Object.defineProperty(globalThis, n, d);
        else delete (globalThis as Record<string, unknown>)[n];
      });
    }
  });

  it("works without window", async () => {
    vi.useFakeTimers();
    vi.spyOn(Math, "random").mockReturnValue(0);
    expect(typeof window).toBe("undefined");
    const { createClient } = await import("./client");
    const ok = createClient({ url: "https://fb.test", key: "k", fetch: async () => new Response(null, { status: 202 }) });
    expect(await ok.send({ message: "hi" })).toEqual({ ok: true });
    const down = createClient({
      url: "https://fb.test",
      key: "k",
      maxAttempts: 1,
      fetch: async () => new Response(JSON.stringify({ error: "queue_full" }), { status: 503 }),
    });
    expect(await down.send({ message: "hi" })).toEqual({ ok: false, error: "queue_full", retryable: true, queued: false });
    expect(down.pending()).toBe(0);
  });

  it("idempotency fallback without randomUUID", async () => {
    const { newId } = await import("./id");
    const v4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
    for (const stub of [{ getRandomValues: globalThis.crypto.getRandomValues.bind(globalThis.crypto) }, undefined]) {
      vi.stubGlobal("crypto", stub);
      const ids = new Set<string>();
      for (let i = 0; i < 100; i++) {
        const id = newId();
        expect(id).toMatch(v4);
        ids.add(id);
      }
      expect(ids.size).toBe(100);
      vi.unstubAllGlobals();
    }
  });
});
