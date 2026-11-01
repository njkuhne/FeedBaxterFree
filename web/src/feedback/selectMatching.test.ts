import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Filters } from "../lib/types";
import { selectMatching } from "./selectMatching";
import { fb, mockApi } from "./testHelpers";

const filters: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const make = (from: number, n: number) => Array.from({ length: n }, (_, i) => fb({ id: `fb_${String(from + i).padStart(4, "0")}` }));

beforeEach(() => vi.unstubAllGlobals());

describe("selectMatching", () => {
  it("pages ids up to the 500 cap", async () => {
    const { calls } = mockApi((c) => {
      const cursor = new URL(c.path, "http://x").searchParams.get("cursor");
      const n = Number(cursor);
      return { body: { items: make(n, 200), nextCursor: String(n + 200) } };
    });
    const loaded = { items: make(0, 50), nextCursor: "50" };
    const r = await selectMatching(filters, loaded);
    expect(r.items).toHaveLength(500);
    expect(r.capped).toBe(true);
    expect(new Set(r.items.map((i) => i.id)).size).toBe(500);
    expect(calls.length).toBe(3);
    expect(calls.every((c) => c.path.includes("limit=200"))).toBe(true);
  });

  it("stops when the cursor runs out", async () => {
    const { calls } = mockApi((c) => {
      const cursor = new URL(c.path, "http://x").searchParams.get("cursor");
      return cursor === "50"
        ? { body: { items: make(50, 200), nextCursor: "250" } }
        : { body: { items: make(250, 30), nextCursor: null } };
    });
    const r = await selectMatching(filters, { items: make(0, 50), nextCursor: "50" });
    expect(r.items).toHaveLength(280);
    expect(r.capped).toBe(false);
    expect(calls).toHaveLength(2);
  });
});
