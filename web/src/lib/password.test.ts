import { describe, expect, it } from "vitest";
import { checkNewPassword } from "./password";

describe("checkNewPassword", () => {
  it("checks length first", () => {
    expect(checkNewPassword("short", "short")).toEqual({ field: "new", message: "Use 10 to 256 characters." });
    expect(checkNewPassword("x".repeat(257), "x".repeat(257))?.field).toBe("new");
  });
  it("checks the confirmation", () => {
    expect(checkNewPassword("long enough pw", "long enough px")).toEqual({
      field: "confirm",
      message: "The new passwords do not match.",
    });
  });
  it("accepts a matching password in range", () => {
    expect(checkNewPassword("0123456789", "0123456789")).toBeNull();
    expect(checkNewPassword("x".repeat(256), "x".repeat(256))).toBeNull();
  });
});
