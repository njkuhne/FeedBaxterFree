import { describe, expect, it } from "vitest";
import { defaultFilters, filtersFromQuery, filtersToQuery, toApiParams } from "./filters";
import type { Filters } from "./types";

describe("filters", () => {
  it("round-trips query and filters", () => {
    const f: Filters = { status: "reviewed", tag: "bug", channel: "ch_1", q: "export button", userId: "" };
    const qs = filtersToQuery(f).toString();
    expect(filtersFromQuery(new URLSearchParams(qs))).toEqual(f);
    const all: Filters = { ...defaultFilters, status: "all" };
    expect(filtersFromQuery(filtersToQuery(all))).toEqual(all);
  });

  it("omits the default status new from the URL", () => {
    expect(filtersToQuery(defaultFilters).toString()).toBe("");
    expect(filtersFromQuery(new URLSearchParams("")).status).toBe("new");
    expect(filtersFromQuery(new URLSearchParams("status=bogus")).status).toBe("new");
  });

  it("sends no status for all", () => {
    expect(toApiParams({ ...defaultFilters, status: "all" }).has("status")).toBe(false);
    expect(toApiParams(defaultFilters).get("status")).toBe("new");
  });

  it("drops empty values", () => {
    expect(toApiParams({ ...defaultFilters, q: "   " }).toString()).toBe("status=new");
    expect(filtersToQuery({ ...defaultFilters, tag: "", q: "" }).toString()).toBe("");
  });

  it("trims and limits the search text and passes the user id", () => {
    const p = toApiParams({ ...defaultFilters, q: ` ${"a".repeat(300)} `, userId: "cust 1" });
    expect(p.get("q")).toHaveLength(200);
    expect(p.get("userId")).toBe("cust 1");
  });
});
