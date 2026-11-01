import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Highlight, splitMatches } from "./Highlight";

describe("Highlight", () => {
  it("splitMatches finds every case-insensitive match", () => {
    expect(splitMatches("Safari crashes in safari, SAFARI!", "  Safari ")).toEqual([
      { text: "Safari", match: true },
      { text: " crashes in ", match: false },
      { text: "safari", match: true },
      { text: ", ", match: false },
      { text: "SAFARI", match: true },
      { text: "!", match: false },
    ]);
    expect(splitMatches("nothing here", "zzz")).toEqual([{ text: "nothing here", match: false }]);
    expect(splitMatches("plain", "")).toEqual([{ text: "plain", match: false }]);
    // Characters that mean something in a regular expression are just characters.
    expect(splitMatches("a.b (c)", ".b (")).toEqual([
      { text: "a", match: false },
      { text: ".b (", match: true },
      { text: "c)", match: false },
    ]);
  });

  it("an injected tag in a message renders literally", () => {
    const message = "Click <b>here</b> now";
    const { container } = render(<p>{<Highlight text={message} query="here" />}</p>);
    expect(container.querySelector("b")).toBeNull();
    expect(container.textContent).toBe(message);
    const marks = container.querySelectorAll("mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]?.textContent).toBe("here");
  });
});
