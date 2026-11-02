import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { listKey, setList } from "../feedback/listStore";
import { fb, labelOf, mockApi, renderPage, renderRoutes, type Call } from "../feedback/testHelpers";
import { mockMatchMedia } from "../test-media";
import type { FeedbackDetail as Detail, Role, Status } from "../lib/types";
import FeedbackDetail from "./FeedbackDetail";

let role: Role = "admin";
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role, mustChangePassword: false } }),
}));

function detail(over: Partial<Detail> = {}): Detail {
  return { ...fb({ id: "fb_1" }), history: [], ...over };
}

function routes(c: Call, d: Detail) {
  if (c.path === "/api/feedback/fb_1") return { body: d };
  if (c.path === "/api/tags") return { body: { items: [] } };
  if (c.path === "/api/channels") return { body: { items: [{ id: "ch_1", name: "Default", createdAt: "x" }] } };
  return undefined;
}

function deferred() {
  let release: () => void = () => undefined;
  const promise = new Promise<void>((resolve) => {
    release = resolve;
  });
  return { promise, release };
}

function setup(d: Detail) {
  mockApi((c) => routes(c, d));
  return renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
}

beforeEach(() => {
  vi.unstubAllGlobals();
  role = "admin";
});

describe("FeedbackDetail", () => {
  it("keeps line breaks in the message through the message class", async () => {
    setup(detail({ message: "Line one\nLine two" }));
    const msg = await screen.findByText(/Line one/);
    expect(msg.className).toBe("message");
    expect(msg.tagName).toBe("P");
    expect(msg.textContent).toBe("Line one\nLine two");
  });

  it("renders a script string as text", async () => {
    const { container } = setup(detail({ message: "<script>alert(1)</script>" }));
    await screen.findByText("<script>alert(1)</script>");
    expect(container.querySelector("script")).toBeNull();
  });

  it("follows the primary button rule", async () => {
    const first = setup(detail({ status: "new" }));
    expect(await screen.findByRole("button", { name: "Mark reviewed" })).toBeTruthy();
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(1);
    expect(labelOf(document.querySelector(".btn-primary"))).toBe("Mark reviewed");
    first.unmount();
    const second = setup(detail({ status: "reviewed" }));
    await screen.findByRole("button", { name: "Mark actioned" });
    expect(labelOf(document.querySelector(".btn-primary"))).toBe("Mark actioned");
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(1);
    second.unmount();
    setup(detail({ status: "actioned" }));
    await screen.findByRole("button", { name: "Mark new" });
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(0);
  });

  it("status labels and order are the same for every status", async () => {
    const labels = ["Mark reviewed", "Mark actioned", "Mark ignored", "Mark new"];
    const order: Status[] = ["reviewed", "actioned", "ignored", "new"];
    for (const status of ["new", "reviewed", "actioned", "ignored"] as const) {
      const view = setup(detail({ status }));
      await screen.findByText("Message fb_1");
      const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>(".status-actions button"));
      expect(buttons.map((b) => labelOf(b))).toEqual(labels);
      expect(buttons.map((b) => b.disabled)).toEqual(order.map((s) => s === status));
      expect(document.querySelectorAll(".btn-primary").length).toBeLessThanOrEqual(1);
      view.unmount();
    }
  });

  it("has no visible title and a hidden h1 naming the user", async () => {
    setup(detail({ userId: "usr_8f2a91" }));
    await screen.findByText("Message fb_1");
    const h1 = await screen.findByRole("heading", { level: 1 });
    expect(h1.textContent).toBe("Feedback from usr_8f2a91");
    expect(h1.classList.contains("visually-hidden")).toBe(true);
    expect(document.querySelectorAll("h1:not(.visually-hidden)")).toHaveLength(0);
    cleanup();
    setup(detail({ userId: null }));
    await screen.findByText("Message fb_1");
    expect((await screen.findByRole("heading", { level: 1 })).textContent).toBe("Feedback with no user ID");
  });

  it("a double click sends one request", async () => {
    const gate = deferred();
    const { calls } = mockApi((c) => {
      if (c.path === "/api/feedback/fb_1" && c.method === "PATCH") {
        return { body: { ...detail({ status: "reviewed" }) }, after: gate.promise };
      }
      return routes(c, detail());
    });
    renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
    const button = await screen.findByRole("button", { name: "Mark reviewed" });
    fireEvent.click(button);
    fireEvent.click(button);
    fireEvent.click(button);
    gate.release();
    await screen.findByText("Marked 1 reviewed");
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(1);
  });

  it("a key pressed during a request is applied after it", async () => {
    const gate = deferred();
    let status: Status = "new";
    const { calls } = mockApi((c) => {
      if (c.path === "/api/feedback/fb_1" && c.method === "PATCH") {
        status = (c.body as { status: Status }).status;
        return { body: detail({ status }), after: status === "reviewed" ? gate.promise : undefined };
      }
      return routes(c, detail({ status }));
    });
    renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
    await screen.findByText("Message fb_1");
    fireEvent.keyDown(document.body, { key: "e" });
    fireEvent.keyDown(document.body, { key: "a" });
    expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(1);
    gate.release();
    await waitFor(() => expect(calls.filter((c) => c.method === "PATCH")).toHaveLength(2));
    expect(calls.filter((c) => c.method === "PATCH").map((c) => c.body)).toEqual([{ status: "reviewed" }, { status: "actioned" }]);
    await waitFor(() => expect(labelOf(document.querySelector(".status-actions button:disabled"))).toBe("Mark actioned"));
  });

  it("hides delete from reviewers and shows it to admins", async () => {
    role = "reviewer";
    const first = setup(detail());
    await screen.findByText("Message fb_1");
    expect(screen.queryByRole("button", { name: "Delete feedback" })).toBeNull();
    first.unmount();
    role = "admin";
    setup(detail());
    expect(await screen.findByRole("button", { name: "Delete feedback" })).toBeTruthy();
  });

  it("renders history with the source and a removed user", async () => {
    setup(
      detail({
        history: [
          { from: "new", to: "reviewed", userId: "us_1", userEmail: "a@example.com", via: "mcp", at: "2026-03-02T10:00:00.000Z" },
          { from: "reviewed", to: "ignored", userId: "us_2", userEmail: null, via: "ui", at: "2026-03-03T10:00:00.000Z" },
        ],
      }),
    );
    expect(await screen.findByText(/Marked reviewed by a@example.com via MCP,/)).toBeTruthy();
    expect(screen.getByText(/Marked ignored by a removed user,/)).toBeTruthy();
    expect(screen.getByText(/^Received$/)).toBeTruthy();
  });

  it("shows metadata values as text and objects as JSON", async () => {
    setup(detail({ metadata: { screen: "export", plan: { tier: "pro" } } }));
    await screen.findByText("export");
    expect(document.querySelector(".meta-json")?.textContent).toBe('{\n  "tier": "pro"\n}');
  });

  it("shows the not-found state", async () => {
    mockApi(() => undefined);
    renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
    await waitFor(() => expect(screen.getByRole("heading", { name: "Feedback not found" })).toBeTruthy());
    expect(screen.getByText("It may have been deleted.")).toBeTruthy();
  });

  it("highlights the query in the message when opened from a search", async () => {
    mockApi((c) => routes(c, detail({ message: "The app will crash on <i>launch</i>" })));
    const filters = { status: "all" as const, tag: "", channel: "", q: "CRASH", userId: "" };
    const { container } = renderRoutes(
      [
        {
          pathname: "/feedback/fb_1",
          state: { from: { kind: "inbox", key: listKey(filters), filters, path: "/?status=all&q=CRASH", label: "Inbox" } },
        },
      ],
      [{ path: "/feedback/:id", element: <FeedbackDetail /> }],
    );
    await waitFor(() => expect(container.querySelector(".message mark")).not.toBeNull());
    const marks = container.querySelectorAll(".message mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]?.textContent).toBe("crash");
    expect(container.querySelector(".message")?.textContent).toBe("The app will crash on <i>launch</i>");
    expect(container.querySelector(".message i")).toBeNull();
  });

  it("t opens the tag editor", async () => {
    setup(detail());
    await screen.findByText("Message fb_1");
    expect(screen.queryByRole("combobox")).toBeNull();
    fireEvent.keyDown(document.body, { key: "t" });
    const input = await screen.findByRole("combobox");
    await waitFor(() => expect(document.activeElement).toBe(input));
    expect(screen.queryByRole("heading", { name: "Tags" })).toBeTruthy();
  });

  it("removing a tag offers Undo and Undo restores it", async () => {
    const d = detail({ tags: ["bug"] });
    const { calls } = mockApi((c) => {
      if (c.path === "/api/feedback/remove-tags" || c.path === "/api/feedback/add-tags") {
        return { body: { updated: 1, notFound: [] } };
      }
      return routes(c, d);
    });
    renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
    fireEvent.click(await screen.findByRole("button", { name: "Remove tag bug" }));
    await screen.findByText("Removed tag bug");
    expect(calls.find((c) => c.path === "/api/feedback/remove-tags")?.body).toEqual({ ids: ["fb_1"], tags: ["bug"] });
    expect(screen.queryByRole("button", { name: "Remove tag bug" })).toBeNull();
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Removed tag bug. Press z to undo."));
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await screen.findAllByText("Restored tag bug on 1 item.");
    expect(calls.find((c) => c.path === "/api/feedback/add-tags")?.body).toEqual({ ids: ["fb_1"], tags: ["bug"] });
    expect(await screen.findByRole("button", { name: "Remove tag bug" })).toBeTruthy();
  });
});

describe("FeedbackDetail on a narrow screen", () => {
  let restore: () => void = () => undefined;
  beforeEach(() => {
    restore = mockMatchMedia(true);
  });
  afterEach(() => restore());

  const group = () => screen.getByRole("group", { name: "Status" });

  function openFromList(d: Detail, opts: { next: boolean }) {
    mockApi((c) => routes(c, d));
    const filters = { status: "new", tag: "", channel: "", q: "", userId: "" } as const;
    const key = listKey(filters);
    const items = opts.next ? [fb({ id: "fb_1" }), fb({ id: "fb_2" })] : [fb({ id: "fb_1" })];
    setList(key, { filters, items, nextCursor: null });
    const from = { kind: "inbox" as const, key, filters, path: "/", label: "Inbox" };
    return renderRoutes(
      [{ pathname: "/feedback/fb_1", state: { from } }],
      [{ path: "/feedback/:id", element: <FeedbackDetail /> }],
    );
  }

  it("narrow detail renders the sticky bar with the four actions in order", async () => {
    setup(detail({ status: "new" }));
    await screen.findByText("Message fb_1");
    expect(document.querySelectorAll(".status-actions")).toHaveLength(1);
    const bar = group();
    expect(bar.classList.contains("status-bar")).toBe(true);
    expect(within(bar).getAllByRole("button").map((b) => b.textContent)).toEqual([
      "Mark reviewed",
      "Mark actioned",
      "Mark ignored",
      "Mark new",
    ]);
    // The bar is the last block of the page, after the footer.
    expect(document.querySelector(".detail")?.lastElementChild).toBe(bar);
  });

  it("the current status is disabled in the sticky bar", async () => {
    setup(detail({ status: "ignored" }));
    await screen.findByText("Message fb_1");
    const disabled = within(group())
      .getAllByRole("button")
      .filter((b) => (b as HTMLButtonElement).disabled)
      .map((b) => b.textContent);
    expect(disabled).toEqual(["Mark ignored"]);
  });

  it("the sticky bar shows Next when a next item exists", async () => {
    openFromList(detail(), { next: true });
    await screen.findByText("Message fb_1");
    const next = within(group()).getByRole("button", { name: "Next" });
    expect(next).toBeTruthy();
    expect(group().classList.contains("status-bar-solo")).toBe(false);
    // The top pager still has its own Next.
    expect(screen.getAllByRole("button", { name: "Next" })).toHaveLength(2);
  });

  it("the sticky bar has no Next without a next item", async () => {
    openFromList(detail(), { next: false });
    await screen.findByText("Message fb_1");
    expect(within(group()).queryByRole("button", { name: "Next" })).toBeNull();
    expect(group().classList.contains("status-bar-solo")).toBe(true);
    expect(within(group()).getAllByRole("button")).toHaveLength(4);
  });

  it("the sticky bar holds the only primary button", async () => {
    setup(detail({ status: "reviewed" }));
    await screen.findByText("Message fb_1");
    const primaries = document.querySelectorAll(".btn-primary");
    expect(primaries).toHaveLength(1);
    expect(group().contains(primaries[0] ?? null)).toBe(true);
    expect(primaries[0]?.textContent).toBe("Mark actioned");
  });

  it("detail meta has no separator text", async () => {
    restore();
    restore = mockMatchMedia(false);
    setup(detail({ userId: "usr_8f2a91", platform: "ios", appVersion: "2.4.1" }));
    await screen.findByText("Message fb_1");
    const meta = document.querySelector(".detail-meta") as HTMLElement;
    expect(meta.tagName).toBe("UL");
    const walker = document.createTreeWalker(meta, NodeFilter.SHOW_TEXT);
    const texts: string[] = [];
    for (let n = walker.nextNode(); n !== null; n = walker.nextNode()) texts.push(n.textContent ?? "");
    expect(texts.length).toBeGreaterThan(4);
    for (const t of texts) expect(/^\s*\u00b7\s*$/.test(t)).toBe(false);
    expect(meta.textContent).not.toContain("\u00b7");
    // Desktop: the status buttons are inline, not a fixed bar.
    expect(document.querySelector(".status-bar")).toBeNull();
  });
});

describe("FeedbackDetail phase 3", () => {
  let undoMedia: () => void = () => undefined;
  const statsBody = (total: number) => ({ total, byStatus: {}, byChannel: [], byTag: [] });
  afterEach(() => {
    undoMedia();
    undoMedia = () => undefined;
  });

  function withStats(d: Detail, total: number | null) {
    return mockApi((c) => {
      if (c.path.startsWith("/api/stats")) return total === null ? undefined : { body: statsBody(total) };
      return routes(c, d);
    });
  }

  function stubClipboard(ok: boolean) {
    const writeText = vi.fn(() => (ok ? Promise.resolve() : Promise.reject(new Error("denied"))));
    Object.defineProperty(navigator, "clipboard", { configurable: true, value: { writeText } });
    return writeText;
  }

  it("Copy message copies the plain text with line breaks", async () => {
    const writeText = stubClipboard(true);
    setup(detail({ message: "Line one\nLine two" }));
    fireEvent.click(await screen.findByRole("button", { name: "Copy message" }));
    await screen.findByText("Message copied.");
    expect(writeText).toHaveBeenCalledWith("Line one\nLine two");
  });

  it("Copy message reports a failure", async () => {
    stubClipboard(false);
    // The textarea fallback has no execCommand in jsdom, so this fails too.
    Object.defineProperty(document, "execCommand", { configurable: true, value: () => false });
    setup(detail());
    fireEvent.click(await screen.findByRole("button", { name: "Copy message" }));
    await screen.findByText("Could not copy. Select the text and copy it manually.");
  });

  it("Copy link copies the item URL", async () => {
    const writeText = stubClipboard(true);
    setup(detail());
    fireEvent.click(await screen.findByRole("button", { name: "Copy link" }));
    await screen.findByText("Link copied.");
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/feedback/fb_1`);
  });

  it("nested metadata is pretty-printed as text", async () => {
    setup(detail({ metadata: { plan: { tier: "pro", seats: [1, 2] }, count: 3, ok: true, gone: null } }));
    await screen.findByText("count");
    expect(document.querySelector("pre.meta-json")?.textContent).toBe(JSON.stringify({ tier: "pro", seats: [1, 2] }, null, 2));
    const facts = document.querySelector("dl.facts") as HTMLElement;
    expect(within(facts).getByText("3")).toBeTruthy();
    expect(within(facts).getByText("true")).toBeTruthy();
    expect(within(facts).getByText("null")).toBeTruthy();
  });

  it("metadata never renders HTML", async () => {
    setup(detail({ metadata: { note: "<b>x</b>", more: { html: "<i>y</i>" } } }));
    expect(await screen.findByText("<b>x</b>")).toBeTruthy();
    expect(document.querySelector(".facts b, .facts i")).toBeNull();
    expect(document.querySelector(".meta-json")?.textContent).toContain("<i>y</i>");
  });

  it("history lines are sentences", async () => {
    setup(
      detail({
        history: [
          { from: "new", to: "reviewed", userId: "us_1", userEmail: "a@example.com", via: "mcp", at: new Date(2026, 10, 1, 15, 57).toISOString() },
        ],
      }),
    );
    await screen.findByText(/Marked reviewed by a@example.com via MCP,/);
    const lines = Array.from(document.querySelectorAll(".history li")).map((li) => li.textContent);
    expect(lines).toEqual([expect.stringMatching(/^Received \d{1,2} \w{3} \d{4} \d{2}:\d{2}$/), "Marked reviewed by a@example.com via MCP, 1 Nov 2026 15:57"]);
    expect(document.querySelectorAll(".history li time")).toHaveLength(2);
  });

  it("More from this user shows the count of other items", async () => {
    const { calls } = withStats(detail({ userId: "usr_8f2a91" }), 4);
    render_();
    const link = await screen.findByRole("link", { name: "More from this user (3)" });
    expect(link.getAttribute("href")).toBe("/user/usr_8f2a91");
    expect(calls.filter((c) => c.path.startsWith("/api/stats?userId=usr_8f2a91"))).toHaveLength(1);
  });

  it("More from this user is hidden when there are no other items", async () => {
    withStats(detail({ userId: "usr_8f2a91" }), 1);
    render_();
    await screen.findByText("Message fb_1");
    await waitFor(() => expect(screen.queryByRole("link", { name: /More from this user/ })).toBeNull());
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByRole("link", { name: /More from this user/ })).toBeNull();
  });

  it("More from this user shows without a count when the count is unknown", async () => {
    withStats(detail({ userId: "usr_8f2a91" }), null);
    render_();
    expect(await screen.findByRole("link", { name: "More from this user" })).toBeTruthy();
  });

  it("Delete feedback sits in a Data removal section", async () => {
    setup(detail());
    const region = await screen.findByRole("region", { name: "Data removal" });
    expect(within(region).getByText("Deleting is permanent. Deleted feedback still counts toward this month's limit.")).toBeTruthy();
    expect(within(region).getByRole("button", { name: "Delete feedback" })).toBeTruthy();
    // It is the last block on desktop.
    expect(document.querySelector(".detail")?.lastElementChild).toBe(region);
  });

  it("status actions show key hints on desktop and not on narrow", async () => {
    const first = setup(detail({ status: "new" }));
    await screen.findByText("Message fb_1");
    const hints = Array.from(document.querySelectorAll(".status-actions .kbd-hint")).map((k) => k.textContent);
    expect(hints).toEqual(["e", "a", "i", "n"]);
    expect(screen.getByRole("button", { name: "Mark reviewed" }).getAttribute("aria-keyshortcuts")).toBe("e");
    first.unmount();
    undoMedia = mockMatchMedia(true);
    setup(detail({ status: "new" }));
    await screen.findByText("Message fb_1");
    expect(document.querySelectorAll(".status-actions .kbd-hint")).toHaveLength(0);
    expect(document.querySelectorAll(".status-actions [aria-keyshortcuts]")).toHaveLength(0);
  });
});

function render_() {
  return renderPage(<FeedbackDetail />, { at: "/feedback/fb_1", route: "/feedback/:id" });
}
