import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Filters } from "../lib/types";
import FeedbackDetail from "../pages/FeedbackDetail";
import { Rail } from "../shell/Rail";
import { AppShortcuts } from "./shortcuts";
import { FeedbackList } from "./FeedbackList";
import { fb, mockApi, renderPage } from "./testHelpers";
import { clearFeedbackCache } from "./useFeedbackList";

let signedIn = true;
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({
    user: signedIn ? { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false } : null,
    signOut: () => Promise.resolve(),
  }),
}));

const filters: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const items = [fb({ id: "fb_2", message: "Second" }), fb({ id: "fb_1", message: "First" })];

function api() {
  return mockApi((c) => {
    if (c.method === "GET" && c.path.startsWith("/api/feedback?")) return { body: { items, nextCursor: null } };
    if (c.method === "GET" && c.path === "/api/feedback/fb_1") return { body: { ...items[1], history: [] } };
    if (c.method === "PATCH") return { body: { ...items[0], status: "reviewed" } };
    if (c.path === "/api/tags" || c.path === "/api/channels") return { body: { items: [] } };
    return undefined;
  });
}

function inbox() {
  return renderPage(
    <>
      <input aria-label="Search" />
      <FeedbackList
        filters={filters}
        showChannel={false}
        origin="inbox"
        originLabel="Inbox"
        renderEmpty={() => <p>Nothing here</p>}
      />
    </>,
  );
}

const dialog = () => screen.getByRole("dialog", { name: "Keyboard shortcuts" });

beforeEach(() => {
  signedIn = true;
  clearFeedbackCache();
  vi.unstubAllGlobals();
});

describe("shortcuts dialog", () => {
  it("? opens the shortcuts dialog from the inbox", async () => {
    api();
    inbox();
    await screen.findByRole("link", { name: "Second" });
    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.keyDown(document.body, { key: "?" });
    expect(dialog()).toBeTruthy();
  });

  it("? opens the shortcuts dialog from the detail page", async () => {
    api();
    renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
    await screen.findByText("First");
    fireEvent.keyDown(document.body, { key: "?" });
    expect(dialog()).toBeTruthy();
  });

  it("? typed in the search box does not open the dialog", async () => {
    api();
    inbox();
    await screen.findByRole("link", { name: "Second" });
    fireEvent.keyDown(screen.getByLabelText("Search"), { key: "?" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("lists every group with its keys", async () => {
    api();
    inbox();
    await screen.findByRole("link", { name: "Second" });
    fireEvent.keyDown(document.body, { key: "?" });
    const d = within(dialog());
    expect(d.getAllByRole("heading", { level: 3 }).map((h) => h.textContent)).toEqual([
      "List",
      "Status (list and detail)",
      "Detail",
      "Selection",
      "Undo",
      "Help",
      "Settings",
    ]);
    const keys = Array.from(dialog().querySelectorAll("kbd")).map((k) => k.textContent);
    for (const k of ["j", "k", "↓", "↑", "Enter", "/", "e", "r", "a", "i", "n", "t", "u", "Escape", "x", "Shift", "z", "?"]) {
      expect(keys).toContain(k);
    }
    expect(d.getByText("Mark reviewed: read, still open")).toBeTruthy();
    expect(d.getByText("Mark actioned: something was done about it")).toBeTruthy();
    expect(d.getByText("Mark new: reopen it")).toBeTruthy();
    expect(d.getByText("Select the next or previous item too")).toBeTruthy();
    expect(d.getByRole("button", { name: "Close" })).toBeTruthy();
  });

  it("the advance switch persists", async () => {
    api();
    inbox();
    await screen.findByRole("link", { name: "Second" });
    fireEvent.keyDown(document.body, { key: "?" });
    const sw = within(dialog()).getByRole("switch", { name: "Advance to the next item after a decision" }) as HTMLInputElement;
    expect(sw.checked).toBe(true);
    fireEvent.click(sw);
    expect(sw.checked).toBe(false);
    expect(window.localStorage.getItem("fbx.pref.advance")).toBe("0");
    fireEvent.click(sw);
    expect(window.localStorage.getItem("fbx.pref.advance")).toBe("1");
  });

  it("the single-key switch turns letter shortcuts off", async () => {
    const { calls } = api();
    inbox();
    const second = await screen.findByRole("link", { name: "Second" });
    fireEvent.keyDown(document.body, { key: "?" });
    fireEvent.click(within(dialog()).getByRole("switch", { name: "Single-key shortcuts" }));
    expect(window.localStorage.getItem("fbx.pref.singleKeys")).toBe("0");
    fireEvent.click(within(dialog()).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    second.focus();
    fireEvent.keyDown(second, { key: "e" });
    fireEvent.keyDown(second, { key: "x" });
    await new Promise((r) => setTimeout(r, 20));
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(0);
    expect(screen.queryByText("1 selected")).toBeNull();
    // ? and the arrow keys still work.
    fireEvent.keyDown(second, { key: "ArrowDown" });
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("link", { name: "First" })));
    fireEvent.keyDown(document.body, { key: "?" });
    expect(dialog()).toBeTruthy();
  });

  it("the rail button opens the dialog", async () => {
    api();
    renderPage(<Rail />);
    fireEvent.click(await screen.findByRole("button", { name: "Keyboard shortcuts" }));
    expect(dialog()).toBeTruthy();
  });

  it("? does nothing on the login page", () => {
    signedIn = false;
    render(
      <AppShortcuts>
        <h1>Sign in</h1>
      </AppShortcuts>,
    );
    fireEvent.keyDown(document.body, { key: "?" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});
