import { describe, expect, it } from "vitest";
import { cursorReducer, initialCursorState, type CursorState } from "./cursorList";

interface Item {
  id: string;
  n: number;
}
const item = (id: string, n = 0): Item => ({ id, n });
const start = (): CursorState<Item> => initialCursorState<Item>();

describe("cursorReducer", () => {
  it("appends and dedupes by id", () => {
    let s = cursorReducer(start(), { type: "reset" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [item("a"), item("b")], nextCursor: "c1" });
    s = cursorReducer(s, { type: "loadMore" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [item("b"), item("c")], nextCursor: null });
    expect(s.items.map((i) => i.id)).toEqual(["a", "b", "c"]);
    expect(s.loading).toBe(false);
  });

  it("ignores a stale request", () => {
    let s = cursorReducer(start(), { type: "reset" });
    const stale = s.requestId;
    s = cursorReducer(s, { type: "reset" });
    s = cursorReducer(s, { type: "loaded", requestId: stale, items: [item("old")], nextCursor: "x" });
    expect(s.items).toEqual([]);
    expect(s.loading).toBe(true);
    s = cursorReducer(s, { type: "failed", requestId: stale, error: "late" });
    expect(s.error).toBeNull();
  });

  it("reset clears items, cursor and error", () => {
    let s = cursorReducer(start(), { type: "reset" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [item("a")], nextCursor: "c" });
    s = cursorReducer(s, { type: "failed", requestId: s.requestId, error: "boom" });
    expect(s.error).toBe("boom");
    s = cursorReducer(s, { type: "reset" });
    expect(s).toMatchObject({ items: [], nextCursor: null, error: null, loading: true });
  });

  it("removes and replaces items", () => {
    let s = cursorReducer(start(), { type: "reset" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [item("a"), item("b"), item("c")], nextCursor: null });
    s = cursorReducer(s, { type: "remove", ids: ["a", "c"] });
    expect(s.items.map((i) => i.id)).toEqual(["b"]);
    s = cursorReducer(s, { type: "replace", item: item("b", 9) });
    expect(s.items[0]?.n).toBe(9);
  });

  it("a null nextCursor ends the list", () => {
    let s = cursorReducer(start(), { type: "reset" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [item("a")], nextCursor: "more" });
    expect(s.nextCursor).toBe("more");
    s = cursorReducer(s, { type: "loadMore" });
    s = cursorReducer(s, { type: "loaded", requestId: s.requestId, items: [], nextCursor: null });
    expect(s.nextCursor).toBeNull();
  });
});
