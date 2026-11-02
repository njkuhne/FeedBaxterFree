import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Feedback, Filters } from "../lib/types";
import { FeedbackList } from "./FeedbackList";
import { fb, mockApi, renderPage, type Call, type Answer } from "./testHelpers";
import { mockMatchMedia } from "../test-media";
import { clearFeedbackCache } from "./useFeedbackList";

const filters: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const items: Feedback[] = [
  fb({ id: "fb_3", message: "Third", receivedAt: "2026-03-03T09:00:00.000Z" }),
  fb({ id: "fb_2", message: "Second", receivedAt: "2026-03-02T09:00:00.000Z" }),
  fb({ id: "fb_1", message: "First", receivedAt: "2026-03-01T09:00:00.000Z" }),
];

function api(extra?: (c: Call) => { status?: number; body?: unknown } | undefined) {
  return mockApi((c) => {
    const own = extra?.(c);
    if (own) return own;
    if (c.method === "GET" && c.path.startsWith("/api/feedback")) return { body: { items, nextCursor: null } };
    if (c.method === "PATCH") {
      const id = c.path.split("/").pop() ?? "";
      const found = items.find((i) => i.id === id);
      return { body: { ...found, status: (c.body as { status: string }).status } };
    }
    if (c.path === "/api/feedback/status") {
      const ids = (c.body as { ids: string[] }).ids;
      return { body: { updated: ids.length, notFound: [] } };
    }
    return undefined;
  });
}

function setup(searchRef?: ReturnType<typeof createRef<HTMLInputElement>>) {
  return renderPage(
    <>
      <input aria-label="Search" ref={searchRef} />
      <FeedbackList
        filters={filters}
        showChannel={false}
        origin="inbox"
        originLabel="Inbox"
        searchRef={searchRef}
        renderEmpty={() => <p>Nothing here</p>}
      />
    </>,
  );
}

const link = (name: string) => screen.getByRole("link", { name });

beforeEach(() => {
  clearFeedbackCache();
  vi.unstubAllGlobals();
});

describe("FeedbackList", () => {
  it("renders rows as list items with the preview as the link name", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    expect(screen.getAllByRole("listitem")).toHaveLength(3);
    expect(link("Second")).toBeTruthy();
  });

  it("moves focus with j and k", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    fireEvent.keyDown(link("Second"), { key: "k" });
    await waitFor(() => expect(document.activeElement).toBe(link("Third")));
  });

  it("toggles selection with x, shows the bulk bar and clears with Escape", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    expect(await screen.findByText("1 selected")).toBeTruthy();
    expect(screen.getByRole("toolbar", { name: "Bulk actions" })).toBeTruthy();
    fireEvent.keyDown(third, { key: "Escape" });
    await waitFor(() => expect(screen.queryByText("1 selected")).toBeNull());
  });

  it("marks the current row reviewed with e, removes it and announces", async () => {
    const { calls } = api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "e" });
    await waitFor(() => expect(screen.queryByRole("link", { name: "Third" })).toBeNull());
    const patch = calls.find((c) => c.method === "PATCH");
    expect(patch?.path).toBe("/api/feedback/fb_3");
    expect(patch?.body).toEqual({ status: "reviewed" });
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Marked 1 as reviewed. Press z to undo."));
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
  });

  it("applies a to the whole selection through the bulk endpoint", async () => {
    const { calls } = api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    fireEvent.keyDown(third, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    fireEvent.keyDown(link("Second"), { key: "x" });
    fireEvent.keyDown(link("Second"), { key: "a" });
    await waitFor(() => expect(calls.some((c) => c.path === "/api/feedback/status")).toBe(true));
    const bulk = calls.find((c) => c.path === "/api/feedback/status");
    expect(bulk?.body).toEqual({ ids: ["fb_3", "fb_2"], status: "actioned" });
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Marked 2 as actioned. Press z to undo."));
  });

  it("ignores shortcuts while the search input has focus", async () => {
    const { calls } = api();
    const ref = createRef<HTMLInputElement>();
    setup(ref);
    await screen.findByRole("link", { name: "Third" });
    const input = screen.getByLabelText("Search");
    input.focus();
    fireEvent.keyDown(input, { key: "e" });
    fireEvent.keyDown(input, { key: "x" });
    expect(calls.some((c) => c.method === "PATCH")).toBe(false);
    expect(screen.queryByText("1 selected")).toBeNull();
  });

  it("handles j and x from the page body, landing on the current row first", async () => {
    api();
    const ref = createRef<HTMLInputElement>();
    setup(ref);
    const first = await screen.findByRole("link", { name: "Third" });
    (document.activeElement as HTMLElement | null)?.blur();
    fireEvent.keyDown(document.body, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(first));
    (document.activeElement as HTMLElement | null)?.blur();
    fireEvent.keyDown(document.body, { key: "x" });
    expect(await screen.findByText("1 selected")).toBeTruthy();
  });

  it("keeps j working while a row checkbox has focus", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    const box = screen.getByLabelText("Select: Third");
    box.focus();
    fireEvent.keyDown(box, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
  });

  it("shows the empty state when there is nothing", async () => {
    mockApi(() => ({ body: { items: [], nextCursor: null } }));
    setup();
    expect(await screen.findByText("Nothing here")).toBeTruthy();
  });

  it("shows an undo toast after a single status key and Undo restores the row in place", async () => {
    const { calls } = api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "e" });
    await waitFor(() => expect(screen.queryByRole("link", { name: "Third" })).toBeNull());
    expect(screen.getByText("Marked 1 reviewed")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(link("Third")).toBeTruthy());
    const restore = calls.find((c) => c.path === "/api/feedback/status");
    expect(restore?.body).toEqual({ ids: ["fb_3"], status: "new" });
    // Back at the top, where it was, not appended.
    expect(screen.getAllByRole("link").map((a) => a.textContent)).toEqual(["Third", "Second", "First"]);
    await waitFor(() => expect(document.activeElement).toBe(link("Third")));
    await screen.findAllByText("Restored 1 item.");
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("bulk undo sends one status call per original status", async () => {
    const mixed: Feedback[] = [
      fb({ id: "fb_3", message: "Third", status: "new", receivedAt: "2026-03-03T09:00:00.000Z" }),
      fb({ id: "fb_2", message: "Second", status: "reviewed", receivedAt: "2026-03-02T09:00:00.000Z" }),
      fb({ id: "fb_1", message: "First", status: "new", receivedAt: "2026-03-01T09:00:00.000Z" }),
    ];
    const { calls } = mockApi((c) => {
      if (c.method === "GET" && c.path.startsWith("/api/feedback")) return { body: { items: mixed, nextCursor: null } };
      if (c.path === "/api/feedback/status") {
        const ids = (c.body as { ids: string[] }).ids;
        return { body: { updated: ids.length, notFound: [] } };
      }
      return undefined;
    });
    renderPage(
      <FeedbackList
        filters={{ ...filters, status: "all" }}
        showChannel={false}
        origin="inbox"
        originLabel="Inbox"
        renderEmpty={() => <p>Nothing here</p>}
      />,
    );
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    fireEvent.keyDown(third, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    fireEvent.keyDown(link("Second"), { key: "x" });
    fireEvent.keyDown(link("Second"), { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("First")));
    fireEvent.keyDown(link("First"), { key: "x" });
    fireEvent.keyDown(link("First"), { key: "i" });
    await screen.findByText("Marked 3 ignored");
    expect(calls.filter((c) => c.path === "/api/feedback/status")).toHaveLength(1);
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await screen.findAllByText("Restored 3 items.");
    const restores = calls.filter((c) => c.path === "/api/feedback/status").slice(1);
    expect(restores.map((c) => c.body)).toEqual([
      { ids: ["fb_3", "fb_1"], status: "new" },
      { ids: ["fb_2"], status: "reviewed" },
    ]);
  });

  it("z undoes the last change", async () => {
    const { calls } = api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "e" });
    await waitFor(() => expect(screen.queryByRole("link", { name: "Third" })).toBeNull());
    fireEvent.keyDown(document.body, { key: "z" });
    await waitFor(() => expect(link("Third")).toBeTruthy());
    expect(calls.find((c) => c.path === "/api/feedback/status")?.body).toEqual({ ids: ["fb_3"], status: "new" });
    // Nothing left to undo: a second z does nothing.
    fireEvent.keyDown(document.body, { key: "z" });
    expect(calls.filter((c) => c.path === "/api/feedback/status")).toHaveLength(1);
  });

  it("a key pressed during a request is applied after it", async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const { calls } = api((c): Answer | undefined =>
      c.method === "PATCH" && c.path === "/api/feedback/fb_3"
        ? { body: { ...items[0], status: "reviewed" }, after: gate }
        : undefined,
    );
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "e" });
    fireEvent.keyDown(third, { key: "e" });
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(1);
    release();
    await waitFor(() => expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(2));
    // The second press ran against the new current row.
    expect(calls.filter((c) => c.method === "PATCH").map((c) => c.path)).toEqual(["/api/feedback/fb_3", "/api/feedback/fb_2"]);
    await waitFor(() => expect(screen.queryByRole("link", { name: "Second" })).toBeNull());
    expect(screen.queryByRole("link", { name: "Third" })).toBeNull();
  });

  it("shift-click selects a range", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.click(screen.getByLabelText("Select: Third"));
    expect(await screen.findByText("1 selected")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Select: First"), { shiftKey: true });
    expect(await screen.findByText("3 selected")).toBeTruthy();
    expect((screen.getByLabelText("Select: Second") as HTMLInputElement).checked).toBe(true);
    // A plain click toggles just that row.
    fireEvent.click(screen.getByLabelText("Select: Third"));
    expect(await screen.findByText("2 selected")).toBeTruthy();
  });

  it("Shift+x selects a range from the anchor to the current row", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    fireEvent.keyDown(third, { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    fireEvent.keyDown(link("Second"), { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("First")));
    fireEvent.keyDown(link("First"), { key: "X", shiftKey: true });
    expect(await screen.findByText("3 selected")).toBeTruthy();
  });

  it("select all matching selects up to 500 and bulk actions use them", async () => {
    const make = (from: number, n: number) =>
      Array.from({ length: n }, (_, i) =>
        fb({
          id: `fb_${String(1000 - from - i).padStart(4, "0")}`,
          message: `Msg ${from + i}`,
          receivedAt: new Date(Date.UTC(2026, 2, 1) - (from + i) * 1000).toISOString(),
        }),
      );
    const { calls } = mockApi((c) => {
      if (c.method === "GET" && c.path.startsWith("/api/feedback")) {
        const cursor = new URL(c.path, "http://x").searchParams.get("cursor");
        const start = cursor === null ? 0 : Number(cursor);
        const n = cursor === null ? 50 : 200;
        return { body: { items: make(start, n), nextCursor: String(start + n) } };
      }
      if (c.path === "/api/feedback/status") {
        const ids = (c.body as { ids: string[] }).ids;
        return { body: { updated: ids.length, notFound: [] } };
      }
      return undefined;
    });
    renderPage(
      <FeedbackList
        filters={filters}
        showChannel={false}
        origin="inbox"
        originLabel="Inbox"
        count={600}
        renderEmpty={() => <p>Nothing here</p>}
      />,
    );
    await screen.findByRole("link", { name: "Msg 0" });
    fireEvent.click(screen.getByLabelText("Select all on this page"));
    expect(await screen.findByText(/All 50 on this page are selected/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Select the first 500 matching" }));
    await screen.findByText(/The first 500 matching are selected\. Bulk actions handle up to 500 at a time\./);
    expect(screen.getByText("500 selected")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Mark reviewed" }));
    await waitFor(() => expect(calls.some((c) => c.path === "/api/feedback/status")).toBe(true));
    const sent = calls.find((c) => c.path === "/api/feedback/status")?.body as { ids: string[]; status: string };
    expect(sent.status).toBe("reviewed");
    expect(sent.ids).toHaveLength(500);
    expect(new Set(sent.ids).size).toBe(500);
    // The list itself only ever held the first page.
    expect(calls.filter((c) => c.method === "GET" && c.path.includes("limit=200"))).toHaveLength(3);
  });

  it("relative times refresh after a minute", async () => {
    vi.useFakeTimers({ toFake: ["Date", "setInterval", "clearInterval"] });
    try {
      vi.setSystemTime(new Date("2026-03-01T10:00:00.000Z"));
      const recent = [fb({ id: "fb_1", message: "Fresh", receivedAt: "2026-03-01T09:54:30.000Z" })];
      mockApi(() => ({ body: { items: recent, nextCursor: null } }));
      setup();
      await screen.findByRole("link", { name: "Fresh" });
      const time = () => document.querySelector(".row-time")?.textContent;
      expect(time()).toBe("5 min ago");
      act(() => {
        vi.advanceTimersByTime(60_000);
      });
      expect(time()).toBe("6 min ago");
    } finally {
      vi.useRealTimers();
    }
  });

  it("t opens the bulk tag editor for the selection", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    // Nothing selected: t selects the current row first.
    fireEvent.keyDown(third, { key: "t" });
    const input = await screen.findByRole("combobox");
    expect(screen.getByText("1 selected")).toBeTruthy();
    await waitFor(() => expect(document.activeElement).toBe(input));
    // Escape in the empty input closes the suggestions, then the popover.
    fireEvent.keyDown(input, { key: "Escape" });
    fireEvent.keyDown(input, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("combobox")).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Tag…" }));
    // The selection survives, and with a selection t opens the popover for all of it.
    expect(screen.getByText("1 selected")).toBeTruthy();
    fireEvent.click(screen.getByLabelText("Select: Second"));
    expect(await screen.findByText("2 selected")).toBeTruthy();
    link("Third").focus();
    fireEvent.keyDown(link("Third"), { key: "t" });
    expect(await screen.findByRole("combobox")).toBeTruthy();
    expect(screen.getByText("2 selected")).toBeTruthy();
  });

  it("bulk remove tag offers Undo", async () => {
    const tagged: Feedback[] = [
      fb({ id: "fb_3", message: "Third", tags: ["bug"], receivedAt: "2026-03-03T09:00:00.000Z" }),
      fb({ id: "fb_2", message: "Second", tags: ["bug"], receivedAt: "2026-03-02T09:00:00.000Z" }),
      fb({ id: "fb_1", message: "First", receivedAt: "2026-03-01T09:00:00.000Z" }),
    ];
    const { calls } = mockApi((c) => {
      if (c.method === "GET" && c.path.startsWith("/api/feedback")) return { body: { items: tagged, nextCursor: null } };
      if (c.path === "/api/feedback/remove-tags" || c.path === "/api/feedback/add-tags") {
        return { body: { updated: (c.body as { ids: string[] }).ids.length, notFound: [] } };
      }
      return undefined;
    });
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.click(screen.getByLabelText("Select: Third"));
    fireEvent.click(screen.getByLabelText("Select: Second"));
    fireEvent.click(await screen.findByRole("button", { name: "Remove tag…" }));
    fireEvent.click(screen.getByRole("button", { name: "bug (2)" }));
    await screen.findByText("Removed tag bug from 2");
    expect(calls.find((c) => c.path === "/api/feedback/remove-tags")?.body).toEqual({ ids: ["fb_3", "fb_2"], tags: ["bug"] });
    expect(document.querySelectorAll(".pill")).toHaveLength(0);
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await screen.findAllByText("Restored tag bug on 2 items.");
    expect(calls.find((c) => c.path === "/api/feedback/add-tags")?.body).toEqual({ ids: ["fb_3", "fb_2"], tags: ["bug"] });
    await waitFor(() => expect(screen.getAllByRole("link", { name: "Filter by tag bug" })).toHaveLength(2));
  });
});

describe("FeedbackList on a narrow screen", () => {
  let restore: () => void = () => undefined;
  beforeEach(() => {
    restore = mockMatchMedia(true);
  });
  afterEach(() => restore());

  const checkboxes = () => document.querySelectorAll(".rows input[type=checkbox]");

  it("narrow list hides row checkboxes and select-all outside Select mode", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    expect(checkboxes()).toHaveLength(0);
    expect(screen.queryByLabelText("Select all on this page")).toBeNull();
    expect(screen.getByRole("button", { name: "Select" })).toBeTruthy();
    expect(document.querySelectorAll(".row-narrow")).toHaveLength(3);
    // The time is in the meta line, once per row.
    expect(document.querySelectorAll(".row-meta time")).toHaveLength(3);
    expect(document.querySelectorAll("time")).toHaveLength(3);
  });

  it("Select shows the checkboxes and select-all", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    expect(checkboxes()).toHaveLength(3);
    expect(screen.getByLabelText("Select all on this page")).toBeTruthy();
    expect(screen.getByText("Select items")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Select" })).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(checkboxes()[0]));
  });

  it("selecting rows in Select mode shows the bulk bar", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    fireEvent.click(screen.getByLabelText("Select: Third"));
    fireEvent.click(screen.getByLabelText("Select: Second"));
    expect(await screen.findByText("2 selected")).toBeTruthy();
    expect(screen.getByRole("toolbar", { name: "Bulk actions" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Mark reviewed" })).toBeTruthy();
    // Done replaces Clear.
    expect(screen.queryByRole("button", { name: "Clear" })).toBeNull();
  });

  it("Done clears the selection and leaves Select mode", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    fireEvent.click(screen.getByLabelText("Select: Third"));
    await screen.findByText("1 selected");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText("1 selected")).toBeNull();
    expect(checkboxes()).toHaveLength(0);
    const select = screen.getByRole("button", { name: "Select" });
    await waitFor(() => expect(document.activeElement).toBe(select));
  });

  it("x on narrow enters Select mode", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    expect(checkboxes()).toHaveLength(0);
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    expect(await screen.findByText("1 selected")).toBeTruthy();
    expect(checkboxes()).toHaveLength(3);
    fireEvent.keyDown(third, { key: "Escape" });
    await waitFor(() => expect(checkboxes()).toHaveLength(0));
    expect(screen.queryByText("1 selected")).toBeNull();
  });

  it("select all matching is not offered on narrow", async () => {
    const make = (from: number, n: number) =>
      Array.from({ length: n }, (_, i) => fb({ id: `fb_${String(1000 - from - i).padStart(4, "0")}`, message: `Msg ${from + i}` }));
    mockApi((c) => {
      if (c.method === "GET" && c.path.startsWith("/api/feedback")) return { body: { items: make(0, 50), nextCursor: "50" } };
      return undefined;
    });
    renderPage(
      <FeedbackList filters={filters} showChannel={false} origin="inbox" originLabel="Inbox" count={600} renderEmpty={() => <p>Nothing here</p>} />,
    );
    await screen.findByRole("link", { name: "Msg 0" });
    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    fireEvent.click(screen.getByLabelText("Select all on this page"));
    expect(await screen.findByText("50 selected")).toBeTruthy();
    expect(screen.queryByText(/on this page are selected/)).toBeNull();
    expect(screen.queryByRole("button", { name: /matching/ })).toBeNull();
  });
});

describe("FeedbackList keyboard additions", () => {
  const checked = () =>
    Array.from(document.querySelectorAll<HTMLInputElement>('.rows input[type="checkbox"]')).map((c) => c.checked);

  it("Shift+j selects the current and the next row", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "J" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    expect(checked()).toEqual([true, true, false]);
    fireEvent.keyDown(link("Second"), { key: "J" });
    await waitFor(() => expect(document.activeElement).toBe(link("First")));
    expect(checked()).toEqual([true, true, true]);
    expect(await screen.findByText("3 selected")).toBeTruthy();
  });

  it("Shift+k selects upwards", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    fireEvent.keyDown(link("Third"), { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    fireEvent.keyDown(link("Second"), { key: "j" });
    await waitFor(() => expect(document.activeElement).toBe(link("First")));
    fireEvent.keyDown(link("First"), { key: "K" });
    await waitFor(() => expect(document.activeElement).toBe(link("Second")));
    expect(checked()).toEqual([false, true, true]);
    fireEvent.keyDown(link("Second"), { key: "K" });
    await waitFor(() => expect(document.activeElement).toBe(link("Third")));
    expect(checked()).toEqual([true, true, true]);
  });

  it("status buttons in the bulk bar show key hints", async () => {
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    third.focus();
    fireEvent.keyDown(third, { key: "x" });
    const bar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    const reviewed = within(bar).getByRole("button", { name: "Mark reviewed" });
    expect(reviewed.getAttribute("aria-keyshortcuts")).toBe("e");
    expect(reviewed.querySelector(".kbd-hint")?.textContent).toBe("e");
    expect(reviewed.querySelector(".kbd-hint")?.getAttribute("aria-hidden")).toBe("true");
    const keys = Array.from(bar.querySelectorAll(".kbd-hint")).map((k) => k.textContent);
    expect(keys).toEqual(["e", "a", "i"]);
  });

  it("key hints are hidden when single keys are off", async () => {
    window.localStorage.setItem("fbx.pref.singleKeys", "0");
    api();
    setup();
    const third = await screen.findByRole("link", { name: "Third" });
    const box = third.closest("li")?.querySelector<HTMLInputElement>('input[type="checkbox"]');
    fireEvent.click(box as HTMLInputElement);
    const bar = await screen.findByRole("toolbar", { name: "Bulk actions" });
    expect(bar.querySelectorAll(".kbd-hint")).toHaveLength(0);
    expect(bar.querySelectorAll("[aria-keyshortcuts]")).toHaveLength(0);
  });

  it("no shortcuts link below the rows", async () => {
    api();
    setup();
    await screen.findByRole("link", { name: "Third" });
    expect(screen.queryByRole("button", { name: "Keyboard shortcuts" })).toBeNull();
    expect(screen.queryByText("Keyboard shortcuts")).toBeNull();
  });
});

describe("FeedbackList paging", () => {
  const page = (from: number, n: number) =>
    Array.from({ length: n }, (_, i) => fb({ id: `fb_${from + i}`, message: `Row ${from + i}`, receivedAt: `2026-03-01T09:${String(59 - ((from + i) % 60)).padStart(2, "0")}:00.000Z` }));

  function paged() {
    return mockApi((c) => {
      if (c.method === "GET" && c.path.startsWith("/api/feedback")) {
        const more = c.path.includes("cursor=");
        return { body: more ? { items: page(50, 10), nextCursor: null } : { items: page(0, 50), nextCursor: "c1" } };
      }
      return undefined;
    });
  }

  function renderWithCount(count: number | null) {
    return renderPage(
      <FeedbackList filters={filters} showChannel={false} origin="inbox" originLabel="Inbox" count={count} renderEmpty={() => <p>Nothing here</p>} />,
    );
  }

  it("says Showing 50 of 312 when more pages exist", async () => {
    paged();
    renderWithCount(312);
    await screen.findByRole("link", { name: "Row 0" });
    expect(screen.getByText("Showing 50 of 312")).toBeTruthy();
  });

  it("says Showing 50 when the total is unknown", async () => {
    paged();
    renderWithCount(null);
    await screen.findByRole("link", { name: "Row 0" });
    expect(screen.getByText("Showing 50")).toBeTruthy();
  });

  it("auto-loads the next page when the end scrolls into view", async () => {
    const observers: { cb: IntersectionObserverCallback; opts: IntersectionObserverInit | undefined }[] = [];
    class FakeObserver {
      constructor(cb: IntersectionObserverCallback, opts?: IntersectionObserverInit) {
        observers.push({ cb, opts });
      }
      observe() {}
      disconnect() {}
      unobserve() {}
      takeRecords() {
        return [];
      }
    }
    vi.stubGlobal("IntersectionObserver", FakeObserver);
    const { calls } = paged();
    renderWithCount(60);
    await screen.findByRole("link", { name: "Row 0" });
    const obs = observers[observers.length - 1];
    expect(obs?.opts?.rootMargin).toBe("0px 0px 400px 0px");
    act(() => obs?.cb([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver));
    await screen.findByRole("link", { name: "Row 59" });
    expect(calls.filter((c) => c.path.includes("cursor=c1"))).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("keeps the Load more button as a fallback", async () => {
    // jsdom has no IntersectionObserver: nothing loads by itself.
    const { calls } = paged();
    renderWithCount(60);
    await screen.findByRole("link", { name: "Row 0" });
    expect(calls.some((c) => c.path.includes("cursor="))).toBe(false);
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    await screen.findByRole("link", { name: "Row 59" });
  });
});

