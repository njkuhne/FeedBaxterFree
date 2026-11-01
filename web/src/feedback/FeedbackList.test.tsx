import { fireEvent, screen, waitFor } from "@testing-library/react";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Feedback, Filters } from "../lib/types";
import { FeedbackList } from "./FeedbackList";
import { fb, mockApi, renderPage, type Call } from "./testHelpers";
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
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Marked 1 as reviewed."));
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
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Marked 2 as actioned."));
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
});
