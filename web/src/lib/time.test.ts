import { describe, expect, it } from "vitest";
import { absoluteTime, relativeTime, shortDate } from "./time";

const now = new Date(2026, 2, 15, 12, 0, 0); // 15 Mar 2026, local
const ago = (ms: number) => new Date(now.getTime() - ms).toISOString();
const S = 1000;
const M = 60 * S;
const H = 60 * M;
const D = 24 * H;

describe("relativeTime", () => {
  it("covers each bucket", () => {
    expect(relativeTime(ago(10 * S), now)).toBe("just now");
    expect(relativeTime(ago(44 * S), now)).toBe("just now");
    expect(relativeTime(ago(45 * S), now)).toBe("1 min ago");
    expect(relativeTime(ago(5 * M), now)).toBe("5 min ago");
    expect(relativeTime(ago(59 * M), now)).toBe("59 min ago");
    expect(relativeTime(ago(60 * M), now)).toBe("1 h ago");
    expect(relativeTime(ago(23 * H), now)).toBe("23 h ago");
    expect(relativeTime(ago(24 * H), now)).toBe("yesterday");
    expect(relativeTime(ago(47 * H), now)).toBe("yesterday");
    expect(relativeTime(ago(2 * D), now)).toBe("2 days ago");
    expect(relativeTime(ago(6 * D + 23 * H), now)).toBe("6 days ago");
  });

  it("uses day and month after a week, adding the year for another year", () => {
    expect(relativeTime(ago(14 * D), now)).toBe("1 Mar");
    expect(relativeTime(new Date(2025, 11, 31, 8, 0).toISOString(), now)).toBe("31 Dec 2025");
  });

  it("handles the year boundary", () => {
    const jan = new Date(2026, 0, 2, 9, 0, 0);
    expect(relativeTime(new Date(2025, 11, 30, 9, 0).toISOString(), jan)).toBe("3 days ago");
    expect(relativeTime(new Date(2025, 11, 20, 9, 0).toISOString(), jan)).toBe("20 Dec 2025");
  });

  it("treats a future time as just now", () => {
    expect(relativeTime(new Date(now.getTime() + H).toISOString(), now)).toBe("just now");
  });
});

describe("absoluteTime", () => {
  it("formats day, month, year and 24 hour time in local time", () => {
    expect(absoluteTime(new Date(2026, 2, 1, 9, 30).toISOString())).toBe("1 Mar 2026, 09:30");
    expect(absoluteTime(new Date(2026, 11, 25, 18, 5).toISOString())).toBe("25 Dec 2026, 18:05");
  });
});

describe("shortDate", () => {
  it("formats day, month and year in local time", () => {
    expect(shortDate(new Date(2026, 2, 1, 9, 30).toISOString())).toBe("1 Mar 2026");
    expect(shortDate(new Date(2026, 11, 25, 23, 59).toISOString())).toBe("25 Dec 2026");
  });
});
