import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

beforeEach(() => {
  vi.resetModules();
});
afterEach(() => {
  vi.useRealTimers();
});

describe("global", () => {
  it("send before init throws", async () => {
    const g = await import("./global");
    expect(() => g.send({ message: "hi" })).toThrow(/init/);
  });

  it("init then send", async () => {
    vi.useFakeTimers();
    const g = await import("./global");
    const f1 = vi.fn<typeof fetch>(async () => new Response(null, { status: 202 }));
    const f2 = vi.fn<typeof fetch>(async () => new Response(null, { status: 202 }));
    const c = g.init({ url: "https://fb.test", key: "k1", fetch: f1, storage: null });
    expect(typeof c.send).toBe("function");
    expect(await g.send({ message: "hi" })).toEqual({ ok: true });
    expect(f1).toHaveBeenCalledTimes(1);
    g.init({ url: "https://fb.test", key: "k2", fetch: f2, storage: null });
    await g.send({ message: "again" });
    expect(f1).toHaveBeenCalledTimes(1);
    expect(f2).toHaveBeenCalledTimes(1);
  });
});
