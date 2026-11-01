import { beforeEach, describe, expect, it } from "vitest";
import type { Filters } from "../lib/types";
import { applyFeedback, applyToList, clearFeedbackCache, getList, setList, subscribe, type FeedbackEvent } from "./listStore";
import { fb } from "./testHelpers";

const newOnly: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const reviewed: Filters = { ...newOnly, status: "reviewed" };
const at = (day: number) => `2026-03-0${day}T09:00:00.000Z`;
const ids = (l: { items: { id: string }[] }) => l.items.map((i) => i.id);

beforeEach(() => clearFeedbackCache());

describe("applyToList", () => {
  const rows = [fb({ id: "fb_4", receivedAt: at(4) }), fb({ id: "fb_2", receivedAt: at(2) }), fb({ id: "fb_1", receivedAt: at(1) })];

  it("replaces, removes and inserts in sorted order", () => {
    const list = { items: rows, nextCursor: null };
    // Replace in place.
    const edited = applyToList(list, newOnly, [fb({ id: "fb_2", receivedAt: at(2), tags: ["bug"] })]);
    expect(ids(edited)).toEqual(["fb_4", "fb_2", "fb_1"]);
    expect(edited.items[1]?.tags).toEqual(["bug"]);
    // Remove when it no longer matches.
    const removed = applyToList(list, newOnly, [fb({ id: "fb_2", receivedAt: at(2), status: "reviewed" })]);
    expect(ids(removed)).toEqual(["fb_4", "fb_1"]);
    // Insert at its sorted place, newest first, ties broken by id descending.
    const inserted = applyToList(removed, newOnly, [fb({ id: "fb_3", receivedAt: at(3) }), fb({ id: "fb_0", receivedAt: at(1) })]);
    expect(ids(inserted)).toEqual(["fb_4", "fb_3", "fb_1", "fb_0"]);
    // Idempotent.
    const again = applyToList(inserted, newOnly, [fb({ id: "fb_3", receivedAt: at(3) }), fb({ id: "fb_0", receivedAt: at(1) })]);
    expect(again).toEqual(inserted);
    // An item that never matched stays out.
    expect(ids(applyToList(list, newOnly, [fb({ id: "fb_9", receivedAt: at(9), status: "ignored" })]))).toEqual(ids(list));
  });

  it("does not insert rows older than the loaded window", () => {
    const list = { items: rows, nextCursor: "next" };
    const older = fb({ id: "fb_0", receivedAt: "2026-02-01T09:00:00.000Z" });
    expect(ids(applyToList(list, newOnly, [older]))).toEqual(["fb_4", "fb_2", "fb_1"]);
    const inside = fb({ id: "fb_3", receivedAt: at(3) });
    expect(ids(applyToList(list, newOnly, [inside]))).toEqual(["fb_4", "fb_3", "fb_2", "fb_1"]);
    // With no further page, the older row belongs at the end.
    expect(ids(applyToList({ items: rows, nextCursor: null }, newOnly, [older]))).toEqual(["fb_4", "fb_2", "fb_1", "fb_0"]);
  });
});

describe("applyFeedback", () => {
  it("updates every cached list by its own filters", () => {
    const item = fb({ id: "fb_2", receivedAt: at(2) });
    const other = fb({ id: "fb_1", receivedAt: at(1), status: "reviewed" });
    setList("new", { filters: newOnly, items: [item], nextCursor: null });
    setList("reviewed", { filters: reviewed, items: [other], nextCursor: null });
    const events: FeedbackEvent[] = [];
    const off = subscribe((e) => events.push(e));
    applyFeedback([{ ...item, status: "reviewed" }], "change");
    off();
    expect(ids(getList("new") ?? { items: [] })).toEqual([]);
    expect(ids(getList("reviewed") ?? { items: [] })).toEqual(["fb_2", "fb_1"]);
    expect(events).toEqual([{ type: "apply", items: [{ ...item, status: "reviewed" }], reason: "change" }]);
  });
});
