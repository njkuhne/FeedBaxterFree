import { describe, expect, it } from "vitest";
import { fb } from "./testHelpers";
import { userSummary } from "./userSummary";

const now = new Date(2026, 10, 1, 12, 0);
const local = (m: number, d: number, h = 9) => new Date(2026, m, d, h, 0).toISOString();

describe("userSummary", () => {
  it("summarises count, first, latest and versions", () => {
    const items = [
      fb({ id: "b", receivedAt: local(10, 1), platform: "web", appVersion: "2.4.1" }),
      fb({ id: "a", receivedAt: local(9, 28), platform: "ios", appVersion: "3.1.0" }),
    ];
    expect(userSummary(items, 2, true, now)).toBe("2 items · first 28 Oct · latest today · Web 2.4.1, iOS 3.1.0");
    expect(userSummary([items[1]!], null, true, now)).toBe("1 item · first 28 Oct · latest 28 Oct · iOS 3.1.0");
    const yesterday = [fb({ id: "c", receivedAt: local(9, 31), platform: null, appVersion: "1.0" })];
    expect(userSummary(yesterday, 1, true, now)).toContain("latest yesterday · 1.0");
    const old = [fb({ id: "d", receivedAt: new Date(2025, 11, 3).toISOString(), platform: null, appVersion: null })];
    expect(userSummary(old, 1, true, now)).toBe("1 item · first 3 Dec 2025 · latest 3 Dec 2025");
  });

  it("omits first while the list is incomplete", () => {
    const items = [fb({ id: "a", receivedAt: local(10, 1), platform: "web", appVersion: "2.4.1" })];
    expect(userSummary(items, 40, false, now)).toBe("40 items · latest today · Web 2.4.1");
  });

  it("caps versions at three", () => {
    const items = ["1", "2", "3", "4", "5"].map((v, n) =>
      fb({ id: v, receivedAt: local(10, 1, 9 + n), platform: "web", appVersion: v }),
    );
    expect(userSummary(items, 5, true, now)).toContain("Web 5, Web 4, Web 3, +2 more");
  });
});
