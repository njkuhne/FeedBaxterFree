import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it, vi } from "vitest";
import { FeedbackRow, previewFor, previewOf } from "./FeedbackRow";
import { fb } from "./testHelpers";

interface Opts {
  showStatus?: boolean;
  showChannel?: boolean;
  showUser?: boolean;
  current?: boolean;
  q?: string;
  kind?: "inbox" | "user";
  onToggle?: (range: boolean) => void;
}

function row(item = fb({ id: "fb_1" }), o: Opts = {}) {
  return render(
    <MemoryRouter>
      <Routes>
        <Route
          path="/"
          element={
            <ul>
              <FeedbackRow
                item={item}
                showStatus={o.showStatus ?? true}
                showChannel={o.showChannel ?? false}
                showUser={o.showUser ?? true}
                selected={false}
                current={o.current ?? true}
                from={{
                  kind: o.kind ?? "inbox",
                  key: "status=all",
                  filters: { status: "all", tag: "", channel: "", q: o.q ?? "", userId: "" },
                  path: "/",
                  label: "Inbox",
                }}
                onToggle={o.onToggle ?? (() => undefined)}
                onCurrent={() => undefined}
              />
            </ul>
          }
        />
        <Route path="/feedback/:id" element={<p>Opened detail</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("FeedbackRow", () => {
  it("renders HTML in the message as literal text", () => {
    const { container } = row(fb({ id: "fb_1", message: "<b>hi</b> there" }));
    expect(screen.getByRole("link", { name: "<b>hi</b> there" })).toBeTruthy();
    expect(container.querySelector("b")).toBeNull();
  });

  it("shows the status word only on All", () => {
    const first = row(fb({ id: "fb_1", status: "reviewed", userId: null }), { showStatus: true });
    const meta = first.container.querySelector(".row-meta");
    expect(meta?.textContent?.startsWith("Reviewed")).toBe(true);
    expect(meta?.textContent).toContain("No user ID");
    first.unmount();
    const second = row(fb({ id: "fb_1", status: "reviewed" }), { showStatus: false });
    expect(second.container.querySelector(".row-status")).toBeNull();
    expect(second.container.querySelector(".row-meta")?.textContent).not.toContain("Reviewed");
  });

  it("the marker has an accessible label when the word is hidden", () => {
    const first = row(fb({ id: "fb_1", status: "new" }), { showStatus: false });
    expect(screen.getByRole("img", { name: "New" })).toBeTruthy();
    first.unmount();
    row(fb({ id: "fb_1", status: "new" }), { showStatus: true });
    expect(screen.queryByRole("img", { name: "New" })).toBeNull();
  });

  it("hides the channel with one channel or a channel filter", () => {
    // FeedbackList passes showChannel=false in both cases; the row shows it only when asked.
    const first = row(fb({ id: "fb_1", channelName: "Web" }), { showChannel: false });
    expect(screen.queryByText("Web")).toBeNull();
    first.unmount();
    row(fb({ id: "fb_1", channelName: "Web" }), { showChannel: true });
    expect(screen.getByText("Web")).toBeTruthy();
  });

  it("hides the user ID on the per-user page", () => {
    const first = row(fb({ id: "fb_1", userId: "usr_8f2a91" }), { showUser: false });
    expect(screen.queryByText("usr_8f2a91")).toBeNull();
    first.unmount();
    row(fb({ id: "fb_1", userId: "usr_8f2a91" }), { showUser: true });
    expect(screen.getByText("usr_8f2a91")).toBeTruthy();
  });

  it("shows platform display names", () => {
    const first = row(fb({ id: "fb_1", platform: "macos" }));
    expect(screen.getByText("macOS")).toBeTruthy();
    first.unmount();
    row(fb({ id: "fb_1", platform: "Tizen" }));
    expect(screen.getByText("Tizen")).toBeTruthy();
  });

  it("shows up to three tags and a count for the rest", () => {
    row(fb({ id: "fb_1", tags: ["a", "b", "c", "d", "e"] }));
    expect(screen.getByText("c")).toBeTruthy();
    expect(screen.queryByText("d")).toBeNull();
    expect(screen.getByText("+2")).toBeTruthy();
  });

  it("collapses runs of spaces, keeps line breaks and cuts the preview at 300 characters", () => {
    expect(previewOf("  one  \t two\nthree  ")).toBe("one two\nthree");
    expect(previewOf("a\n\n\n\n\nb")).toBe("a\n\nb");
    expect(previewOf("x".repeat(400))).toHaveLength(300);
  });

  it("keeps line breaks in the two-line preview", () => {
    const { container } = row(fb({ id: "fb_1", message: "First line\nSecond line\n\n\n\nThird" }));
    const preview = container.querySelector(".row-preview");
    expect(preview?.textContent).toBe("First line\nSecond line\n\nThird");
  });

  it("a click anywhere on the row opens it, but not on the checkbox", () => {
    const first = row(fb({ id: "fb_1", message: "Open me" }));
    fireEvent.click(first.container.querySelector(".row-meta") as HTMLElement);
    expect(screen.getByText("Opened detail")).toBeTruthy();
    first.unmount();

    const onToggle = vi.fn();
    row(fb({ id: "fb_1", message: "Open me" }), { onToggle });
    fireEvent.click(screen.getByLabelText("Select: Open me"));
    expect(onToggle).toHaveBeenCalledWith(false);
    expect(screen.queryByText("Opened detail")).toBeNull();
  });

  it("marks the current row without focus", () => {
    const first = row(fb({ id: "fb_1" }), { current: true });
    expect(first.container.querySelector("li")?.className).toContain("row-current");
    expect(document.activeElement).toBe(document.body);
    first.unmount();
    const second = row(fb({ id: "fb_1" }), { current: false });
    expect(second.container.querySelector("li")?.className).not.toContain("row-current");
  });

  it("highlights the match in the preview", () => {
    const { container } = row(fb({ id: "fb_1", message: "Crash in Safari when printing <b>now</b>" }), { q: "safari" });
    const marks = container.querySelectorAll(".row-preview mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]?.textContent).toBe("Safari");
    expect(container.querySelector("b")).toBeNull();
    // A match far into a long message is brought into view, with an ellipsis.
    const long = `${"word ".repeat(60)}needle and the rest`;
    const windowed = row(fb({ id: "fb_2", message: long }), { q: "needle" });
    const preview = windowed.container.querySelector(".row-preview");
    expect(preview?.textContent?.startsWith("…")).toBe(true);
    expect(preview?.querySelector("mark")?.textContent).toBe("needle");
    expect(previewFor(long, "needle").length).toBeLessThan(120);
    // No query, no marks and no windowing.
    expect(previewFor(long, "")).toBe(previewOf(long));
  });

  it("tag pills link to the tag filter", () => {
    const inbox = row(fb({ id: "fb_1", tags: ["bug"] }), { q: "safari", current: false });
    const link = screen.getByRole("link", { name: "Filter by tag bug" });
    expect(link.getAttribute("href")).toBe("/?status=all&tag=bug&q=safari");
    expect(link.getAttribute("tabindex")).toBe("-1");
    inbox.unmount();
    const user = row(fb({ id: "fb_1", tags: ["needs review"] }), { kind: "user", current: true });
    void user;
    const other = screen.getByRole("link", { name: "Filter by tag needs review" });
    expect(other.getAttribute("href")).toBe("/?status=all&tag=needs+review");
    expect(other.getAttribute("tabindex")).toBe("0");
    // Clicking the pill does not open the row.
    fireEvent.click(other);
    expect(screen.queryByText("Opened detail")).toBeNull();
  });
});
