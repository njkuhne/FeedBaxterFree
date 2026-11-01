import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { clearFeedbackCache } from "../feedback/useFeedbackList";
import { fb, mockApi, renderPage, type Answer, type Call } from "../feedback/testHelpers";
import type { Feedback } from "../lib/types";
import Inbox from "./Inbox";

vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false } }),
}));

const crash = fb({ id: "fb_2", message: "App crash on launch", status: "reviewed", receivedAt: "2026-03-02T09:00:00.000Z" });
const other = fb({ id: "fb_1", message: "Love the new look", status: "new", receivedAt: "2026-03-01T09:00:00.000Z" });

/** A small server: the stats and lookups the inbox needs, and a list handler per test. */
function server(list: (url: URL) => Answer | Feedback[]) {
  return mockApi((c: Call) => {
    const url = new URL(c.path, "http://x");
    if (url.pathname === "/api/stats") {
      return { body: { total: 2, byStatus: { new: 1, reviewed: 1 }, byChannel: [], byTag: [] } };
    }
    if (url.pathname === "/api/channels") return { body: { items: [{ id: "ch_1", name: "Web", createdAt: "x" }] } };
    if (url.pathname === "/api/tags") return { body: { items: [] } };
    if (c.method === "GET" && url.pathname === "/api/feedback") {
      const r = list(url);
      return Array.isArray(r) ? { body: { items: r, nextCursor: null } } : r;
    }
    return undefined;
  });
}

const feedbackCalls = (calls: Call[]) => calls.filter((c) => c.method === "GET" && c.path.startsWith("/api/feedback?"));
const radio = (name: RegExp) => screen.getByRole("radio", { name }) as HTMLInputElement;
const typeSearch = (text: string) => fireEvent.change(screen.getByRole("searchbox", { name: "Search" }), { target: { value: text } });

beforeEach(() => {
  vi.unstubAllGlobals();
  clearFeedbackCache();
});

describe("Inbox search", () => {
  it("a query searches all statuses and selects All", async () => {
    const { calls } = server((url) => {
      const q = url.searchParams.get("q");
      const status = url.searchParams.get("status");
      return [crash, other].filter(
        (i) => (q === null || i.message.toLowerCase().includes(q)) && (status === null || i.status === status),
      );
    });
    renderPage(<Inbox />);
    await screen.findByRole("link", { name: "Love the new look" });
    expect(radio(/^New/).checked).toBe(true);
    typeSearch("crash");
    await waitFor(() => expect(radio(/^All/).checked).toBe(true), { timeout: 2000 });
    const searched = feedbackCalls(calls).filter((c) => c.path.includes("q=crash"));
    expect(searched.length).toBeGreaterThan(0);
    for (const c of searched) expect(new URL(c.path, "http://x").searchParams.has("status")).toBe(false);
    // The reviewed match is found even though the New tab was active, and says which status it has.
    const row = await screen.findByRole("link", { name: "App crash on launch" });
    expect(row.closest("li")?.textContent).toContain("Reviewed");
    expect(document.querySelector(".row-preview mark")?.textContent).toBe("crash");
  });

  it("clearing the search returns to the previous tab", async () => {
    server((url) => {
      const q = url.searchParams.get("q");
      return [crash].filter((i) => q === null || i.message.includes(q));
    });
    renderPage(<Inbox />, { at: "/?status=reviewed" });
    await screen.findByRole("link", { name: "App crash on launch" });
    expect(radio(/^Reviewed/).checked).toBe(true);
    typeSearch("crash");
    await waitFor(() => expect(radio(/^All/).checked).toBe(true), { timeout: 2000 });
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    await waitFor(() => expect(radio(/^Reviewed/).checked).toBe(true));
    expect((screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement).value).toBe("");
  });

  it("keeps a tab the user picked while searching when the search is cleared", async () => {
    server(() => [crash]);
    renderPage(<Inbox />);
    await screen.findByRole("link", { name: "App crash on launch" });
    typeSearch("crash");
    await waitFor(() => expect(radio(/^All/).checked).toBe(true), { timeout: 2000 });
    fireEvent.click(radio(/^Actioned/));
    await waitFor(() => expect(radio(/^Actioned/).checked).toBe(true));
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    await waitFor(() => expect((screen.getByRole("searchbox", { name: "Search" }) as HTMLInputElement).value).toBe(""));
    expect(radio(/^Actioned/).checked).toBe(true);
  });

  it("a pasted user ID with no message hits offers that user's feedback", async () => {
    const { calls } = server((url) => {
      if (url.searchParams.get("userId") === "usr_8f2a91") return [fb({ id: "fb_9", userId: "usr_8f2a91" })];
      return [];
    });
    renderPage(<Inbox />, { at: "/?status=all&q=usr_8f2a91" });
    const offer = await screen.findByRole("link", { name: "Show feedback from usr_8f2a91" });
    expect(offer.getAttribute("href")).toBe("/user/usr_8f2a91");
    const lookups = feedbackCalls(calls).filter((c) => c.path.includes("userId=usr_8f2a91"));
    expect(lookups).toHaveLength(1);
    expect(new URL(lookups[0]?.path ?? "", "http://x").searchParams.get("limit")).toBe("1");
    expect(new URL(lookups[0]?.path ?? "", "http://x").searchParams.has("status")).toBe(false);
  });

  it("does not offer a user page for text with spaces or an unknown ID", async () => {
    const { calls } = server(() => []);
    renderPage(<Inbox />, { at: "/?status=all&q=two%20words" });
    await screen.findByRole("heading", { name: "No matches" });
    expect(feedbackCalls(calls).some((c) => c.path.includes("userId="))).toBe(false);
    expect(screen.queryByRole("link", { name: /^Show feedback from/ })).toBeNull();
  });

  it("keeps the previous results dimmed while searching", async () => {
    let release: () => void = () => undefined;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    server((url) => {
      if (url.searchParams.get("q") === "crash") return { body: { items: [crash], nextCursor: null }, after: gate };
      return [other];
    });
    renderPage(<Inbox />);
    await screen.findByRole("link", { name: "Love the new look" });
    typeSearch("crash");
    await waitFor(() => expect(screen.getByRole("list", { name: "Feedback" }).getAttribute("aria-busy")).toBe("true"), {
      timeout: 2000,
    });
    // The old rows stay, and there is no loading flash.
    expect(screen.getByRole("link", { name: "Love the new look" })).toBeTruthy();
    expect(screen.queryByText("Loading feedback…")).toBeNull();
    await screen.findByText("Searching…");
    expect(screen.queryByText("Loading feedback…")).toBeNull();
    release();
    await screen.findByRole("link", { name: "App crash on launch" });
    expect(screen.queryByRole("link", { name: "Love the new look" })).toBeNull();
    expect(screen.getByRole("list", { name: "Feedback" }).getAttribute("aria-busy")).toBeNull();
    expect(screen.queryByText("Searching…")).toBeNull();
  });

  it("the filtered-empty state names the filters and shows no Baxter", async () => {
    server(() => []);
    renderPage(<Inbox />, { at: "/?status=all&tag=bug&q=safari" });
    await screen.findByRole("heading", { name: "No matches" });
    expect(screen.getByText('Nothing tagged bug matching "safari".')).toBeTruthy();
    expect(document.querySelector(".empty svg")).toBeNull();
    expect(screen.getByRole("button", { name: "Clear filters" })).toBeTruthy();
  });

  it("shows Baxter only for inbox zero, and plain copy for an empty status tab", async () => {
    server(() => []);
    const first = renderPage(<Inbox />);
    await screen.findByRole("heading", { name: "No new feedback" });
    expect(document.querySelector(".empty svg")).not.toBeNull();
    first.unmount();
    renderPage(<Inbox />, { at: "/?status=reviewed" });
    await screen.findByRole("heading", { name: "No reviewed feedback" });
    expect(screen.getByText("Feedback you mark reviewed appears here.")).toBeTruthy();
    expect(document.querySelector(".empty svg")).toBeNull();
    expect(screen.queryByRole("button", { name: "Clear filters" })).toBeNull();
  });
});

describe("Inbox tab counts", () => {
  it("tab counts update when a tag is chosen", async () => {
    const { calls } = mockApi((c: Call) => {
      const url = new URL(c.path, "http://x");
      if (url.pathname === "/api/stats") {
        return url.searchParams.get("tag") === "bug"
          ? { body: { total: 7, byStatus: { new: 5, reviewed: 2 }, byChannel: [], byTag: [{ name: "bug", count: 7 }] } }
          : { body: { total: 20, byStatus: { new: 11, reviewed: 9 }, byChannel: [], byTag: [] } };
      }
      if (url.pathname === "/api/channels") return { body: { items: [] } };
      if (url.pathname === "/api/tags") return { body: { items: [{ id: "t1", name: "bug", count: 7 }] } };
      if (c.method === "GET" && url.pathname === "/api/feedback") return { body: { items: [other], nextCursor: null } };
      return undefined;
    });
    renderPage(<Inbox />);
    await waitFor(() => expect(radio(/^New/).closest("label")?.textContent).toContain("11"));
    fireEvent.change(screen.getByLabelText("Tag"), { target: { value: "bug" } });
    await waitFor(() => expect(calls.some((c) => c.path === "/api/stats?tag=bug")).toBe(true));
    await waitFor(() => expect(radio(/^New/).closest("label")?.textContent).toContain("5"));
    expect(radio(/^New/).closest("label")?.textContent).not.toContain("11");
    expect(radio(/^Reviewed/).closest("label")?.textContent).toContain("2");
  });
});
