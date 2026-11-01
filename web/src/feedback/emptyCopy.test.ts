import { describe, expect, it } from "vitest";
import type { Filters } from "../lib/types";
import { emptyCopy } from "./emptyCopy";

const base: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };

describe("emptyCopy", () => {
  it("names status-only and filtered empties", () => {
    expect(emptyCopy(base)).toEqual({ heading: "No new feedback", body: "New feedback appears here as it arrives." });
    expect(emptyCopy({ ...base, status: "reviewed" })).toEqual({
      heading: "No reviewed feedback",
      body: "Feedback you mark reviewed appears here.",
    });
    expect(emptyCopy({ ...base, status: "actioned" }).body).toBe("Feedback you mark actioned appears here.");
    expect(emptyCopy({ ...base, status: "ignored" }).body).toBe("Feedback you mark ignored appears here.");
    expect(emptyCopy({ ...base, status: "all" })).toEqual({
      heading: "No feedback",
      body: "Feedback appears here as it arrives.",
    });
    expect(emptyCopy({ ...base, tag: "bug", channel: "ch_1", q: " safari " }, "Web")).toEqual({
      heading: "No matches",
      body: 'Nothing in New tagged bug in channel Web matching "safari".',
    });
    expect(emptyCopy({ ...base, status: "all", tag: "bug", q: "safari" }).body).toBe(
      'Nothing tagged bug matching "safari".',
    );
    expect(emptyCopy({ ...base, status: "reviewed", q: "x" }).body).toBe('Nothing in Reviewed matching "x".');
  });
});
