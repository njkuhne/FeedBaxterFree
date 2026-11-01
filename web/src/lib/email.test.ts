import { describe, expect, it } from "vitest";
import { looksLikeEmail } from "./email";

describe("looksLikeEmail", () => {
  it("accepts an address", () => expect(looksLikeEmail("a@b.co")).toBe(true));
  it("rejects opaque ids and malformed values", () => {
    for (const s of ["cust-4821", "usr@local", "a @b.co", ""]) expect(looksLikeEmail(s)).toBe(false);
  });
  it("trims surrounding spaces", () => expect(looksLikeEmail("  a@b.co  ")).toBe(true));
});
