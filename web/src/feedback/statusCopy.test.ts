import { describe, expect, it } from "vitest";
import { ACTION_LABEL, addedTagSentence, markedOneSentence, markedSentence, markedToast, validTag, withUndoHint } from "./statusCopy";

describe("statusCopy", () => {
  it("announces singular and plural counts", () => {
    expect(markedSentence(3, "reviewed")).toBe("Marked 3 as reviewed.");
    expect(markedSentence(1, "reviewed")).toBe("Marked 1 as reviewed.");
  });

  it("appends the not-found suffix", () => {
    expect(markedSentence(2, "ignored", 1)).toBe("Marked 2 as ignored. 1 was no longer there.");
    expect(markedSentence(2, "actioned", 3)).toBe("Marked 2 as actioned. 3 were no longer there.");
    expect(markedSentence(2, "new", 0)).toBe("Marked 2 as new.");
  });

  it("has the detail and tag sentences", () => {
    expect(markedOneSentence("reviewed")).toBe("Marked as reviewed.");
    expect(addedTagSentence("bug", 3)).toBe("Added tag bug to 3.");
  });

  it("has the action labels, the toast text and the undo hint", () => {
    expect(ACTION_LABEL.ignored).toBe("Mark ignored");
    expect(ACTION_LABEL.new).toBe("Mark new");
    expect(markedToast(3, "reviewed")).toBe("Marked 3 reviewed");
    expect(markedToast(1, "reviewed")).toBe("Marked 1 reviewed");
    expect(withUndoHint(markedSentence(3, "reviewed"))).toBe("Marked 3 as reviewed. Press z to undo.");
  });

  it("validates tag length", () => {
    expect(validTag("  bug ")).toBe("bug");
    expect(validTag("   ")).toBeNull();
    expect(validTag("x".repeat(65))).toBeNull();
    expect(validTag("x".repeat(64))).toBe("x".repeat(64));
  });
});
