import { describe, expect, it } from "vitest";
import { loginUrl, safeNext } from "./next";

describe("safeNext", () => {
  it("keeps same-origin paths", () => {
    expect(safeNext("/inbox?status=all")).toBe("/inbox?status=all");
    expect(safeNext("/feedback/fb_1")).toBe("/feedback/fb_1");
  });
  it("rejects everything else", () => {
    for (const s of [null, "", "https://evil.test/", "//evil.test", "/\\evil.test", "feedback", "javascript:alert(1)"]) {
      expect(safeNext(s)).toBe("/");
    }
  });
});

describe("loginUrl", () => {
  it("carries the current location", () => {
    expect(loginUrl("/people", "")).toBe("/login?next=%2Fpeople");
    expect(loginUrl("/", "")).toBe("/login");
  });
});
