import { describe, expect, it } from "vitest";
import { groupByStatus } from "./undo";
import { fb } from "./testHelpers";

describe("groupByStatus", () => {
  it("groups ids by previous status", () => {
    const groups = groupByStatus([
      fb({ id: "a", status: "new" }),
      fb({ id: "b", status: "reviewed" }),
      fb({ id: "c", status: "new" }),
      fb({ id: "d", status: "ignored" }),
    ]);
    expect(Array.from(groups)).toEqual([
      ["new", ["a", "c"]],
      ["reviewed", ["b"]],
      ["ignored", ["d"]],
    ]);
    expect(groupByStatus([]).size).toBe(0);
  });
});
