import { describe, expect, it } from "vitest";
import type { HistoryEntry } from "../lib/types";
import { historySentence, historyTime, receivedSentence } from "./historyCopy";

const at = new Date(2026, 10, 1, 15, 57).toISOString();
const entry = (over: Partial<HistoryEntry> = {}): HistoryEntry => ({
  from: "new",
  to: "reviewed",
  userId: "us_1",
  userEmail: "admin@localhost",
  via: "ui",
  at,
  ...over,
});

describe("historyCopy", () => {
  it("writes a UI change as a sentence", () => {
    expect(historyTime(at)).toBe("1 Nov 2026 15:57");
    expect(historySentence(entry())).toBe("Marked reviewed by admin@localhost, 1 Nov 2026 15:57");
    expect(receivedSentence(new Date(2026, 9, 28, 9, 12).toISOString())).toBe("Received 28 Oct 2026 09:12");
  });

  it("adds via MCP only for MCP changes", () => {
    expect(historySentence(entry({ via: "mcp", userEmail: "ai@example.com", to: "actioned" }))).toBe(
      "Marked actioned by ai@example.com via MCP, 1 Nov 2026 15:57",
    );
    expect(historySentence(entry({ via: "ui" }))).not.toContain("MCP");
  });

  it("names a removed user", () => {
    expect(historySentence(entry({ userEmail: null, to: "new" }))).toBe("Marked new by a removed user, 1 Nov 2026 15:57");
  });
});
