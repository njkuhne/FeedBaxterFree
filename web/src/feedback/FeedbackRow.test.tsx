import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { FeedbackRow, previewOf } from "./FeedbackRow";
import { fb } from "./testHelpers";

function row(item = fb({ id: "fb_1" }), showChannel = false) {
  return render(
    <MemoryRouter>
      <ul>
        <FeedbackRow
          item={item}
          showChannel={showChannel}
          selected={false}
          current
          origin="inbox"
          onToggle={() => undefined}
          onCurrent={() => undefined}
        />
      </ul>
    </MemoryRouter>,
  );
}

describe("FeedbackRow", () => {
  it("renders HTML in the message as literal text", () => {
    const { container } = row(fb({ id: "fb_1", message: "<b>hi</b> there" }));
    expect(screen.getByRole("link", { name: "<b>hi</b> there" })).toBeTruthy();
    expect(container.querySelector("b")).toBeNull();
  });

  it("shows the status word first in the metadata line", () => {
    const { container } = row(fb({ id: "fb_1", status: "reviewed", userId: null }));
    const meta = container.querySelector(".row-meta");
    expect(meta?.textContent?.startsWith("Reviewed")).toBe(true);
    expect(meta?.textContent).toContain("No user ID");
  });

  it("shows the channel only when asked", () => {
    const first = row(fb({ id: "fb_1", channelName: "Web" }), false);
    expect(screen.queryByText("Web")).toBeNull();
    first.unmount();
    row(fb({ id: "fb_1", channelName: "Web" }), true);
    expect(screen.getByText("Web")).toBeTruthy();
  });

  it("shows up to three tags and a count for the rest", () => {
    row(fb({ id: "fb_1", tags: ["a", "b", "c", "d", "e"] }));
    expect(screen.getByText("c")).toBeTruthy();
    expect(screen.queryByText("d")).toBeNull();
    expect(screen.getByText("+2")).toBeTruthy();
  });

  it("collapses line breaks in the preview and cuts it at 200 characters", () => {
    expect(previewOf("one\n\ntwo\tthree")).toBe("one two three");
    expect(previewOf("x".repeat(300))).toHaveLength(200);
  });
});
