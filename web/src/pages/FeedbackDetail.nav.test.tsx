import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { useSearchParams } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { FeedbackList } from "../feedback/FeedbackList";
import { clearFeedbackCache, listKey, setList } from "../feedback/listStore";
import type { FromList } from "../feedback/origin";
import { fb, labelOf, mockApi, renderRoutes, type Call } from "../feedback/testHelpers";
import { filtersFromQuery } from "../lib/filters";
import type { Feedback, Filters, Status } from "../lib/types";
import FeedbackDetail from "./FeedbackDetail";

vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false } }),
}));

const day = (n: number) => `2026-03-0${n}T09:00:00.000Z`;
const rows = (): Feedback[] => [
  fb({ id: "fb_3", receivedAt: day(3) }),
  fb({ id: "fb_2", receivedAt: day(2) }),
  fb({ id: "fb_1", receivedAt: day(1) }),
];

const newFilters: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const fromInbox = (filters: Filters = newFilters, path = "/"): FromList => ({
  kind: "inbox",
  key: listKey(filters),
  filters,
  path,
  label: "Inbox",
});

/** A tiny server: statuses persist across requests, lists honour the status filter. */
function server(initial: Feedback[], opts: { nextPage?: Feedback[] } = {}) {
  const store = new Map<string, Feedback>([...initial, ...(opts.nextPage ?? [])].map((i) => [i.id, i]));
  const statusOf = (id: string): Status => store.get(id)?.status ?? "new";
  const handler = (c: Call) => {
    const url = new URL(c.path, "http://x");
    const all = Array.from(store.values()).sort((a, b) => (a.receivedAt < b.receivedAt ? 1 : -1));
    if (c.method === "GET" && url.pathname === "/api/feedback") {
      const status = url.searchParams.get("status");
      const user = url.searchParams.get("userId");
      if (url.searchParams.get("cursor") === "c1") return { body: { items: opts.nextPage ?? [], nextCursor: null } };
      const items = all
        .filter((i) => !(opts.nextPage ?? []).some((n) => n.id === i.id))
        .filter((i) => (status === null || i.status === status) && (user === null || i.userId === user));
      return { body: { items, nextCursor: opts.nextPage ? "c1" : null } };
    }
    if (c.method === "GET" && url.pathname.startsWith("/api/feedback/")) {
      const item = store.get(decodeURIComponent(url.pathname.split("/").pop() ?? ""));
      return item ? { body: { ...item, history: [] } } : undefined;
    }
    if (c.method === "PATCH") {
      const id = decodeURIComponent(c.path.split("/").pop() ?? "");
      const item = store.get(id);
      if (!item) return undefined;
      const next = { ...item, status: (c.body as { status: Status }).status };
      store.set(id, next);
      return { body: next };
    }
    if (c.method === "POST" && c.path === "/api/feedback/status") {
      const { ids, status } = c.body as { ids: string[]; status: Status };
      for (const id of ids) {
        const item = store.get(id);
        if (item) store.set(id, { ...item, status });
      }
      return { body: { updated: ids.length, notFound: [] } };
    }
    if (c.method === "DELETE") {
      store.delete(decodeURIComponent(c.path.split("/").pop() ?? ""));
      return { status: 204 };
    }
    if (c.path === "/api/stats") {
      return { body: { total: 3, byStatus: { new: 3 }, byChannel: [], byTag: [] } };
    }
    if (c.path === "/api/tags") return { body: { items: [] } };
    if (c.path === "/api/channels") return { body: { items: [{ id: "ch_1", name: "Default", createdAt: "x" }] } };
    return undefined;
  };
  return { handler, statusOf };
}

function ListPage() {
  const [params] = useSearchParams();
  return (
    <>
      <h1>Inbox page</h1>
      <FeedbackList
        filters={filtersFromQuery(params)}
        showChannel={false}
        origin="inbox"
        originLabel="Inbox"
        renderEmpty={() => <p>Nothing here</p>}
      />
    </>
  );
}

function UserPage() {
  return (
    <>
      <h1>User page</h1>
      <p>cust-1 feedback</p>
    </>
  );
}

const pages = [
  { path: "/", element: <ListPage /> },
  { path: "/feedback/:id", element: <FeedbackDetail /> },
  { path: "/user/:userId", element: <UserPage /> },
];

const position = () => document.querySelector(".detail-position")?.textContent;

beforeEach(() => {
  clearFeedbackCache();
  vi.unstubAllGlobals();
});
afterEach(() => cleanup());

function openOn(items: Feedback[], opts: { nextCursor?: string | null; from?: FromList } = {}) {
  const from = opts.from ?? fromInbox();
  setList(from.key, { filters: from.filters, items, nextCursor: opts.nextCursor ?? null });
  return from;
}

describe("FeedbackDetail navigation", () => {
  it("shows the position and moves with Next, Previous, j and k", async () => {
    const srv = server(rows());
    mockApi(srv.handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    expect(position()).toBe("2 of 3 in New");
    const next = screen.getByRole("button", { name: "Next" });
    fireEvent.click(next);
    await screen.findByText("Message fb_1");
    expect(position()).toBe("3 of 3 in New");
    expect((screen.getByRole("button", { name: "Next" }) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "Previous" }));
    await screen.findByText("Message fb_2");
    expect(position()).toBe("2 of 3 in New");
    fireEvent.keyDown(document.body, { key: "j" });
    await screen.findByText("Message fb_1");
    fireEvent.keyDown(document.body, { key: "k" });
    await screen.findByText("Message fb_2");
    fireEvent.keyDown(document.body, { key: "k" });
    await screen.findByText("Message fb_3");
    expect(position()).toBe("1 of 3 in New");
    expect((screen.getByRole("button", { name: "Previous" }) as HTMLButtonElement).disabled).toBe(true);
    // Arrow keys keep scrolling the page.
    fireEvent.keyDown(document.body, { key: "ArrowDown" });
    expect(position()).toBe("1 of 3 in New");
  });

  it("Next at the end of the loaded rows fetches the next page", async () => {
    const older = fb({ id: "fb_0", receivedAt: "2026-02-01T09:00:00.000Z" });
    const srv = server(rows().slice(1), { nextPage: [older] });
    const { calls } = mockApi(srv.handler);
    const from = openOn(rows().slice(1), { nextCursor: "c1" });
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_1", state: { from } }], pages);
    await screen.findByText("Message fb_1");
    await waitFor(() => expect(position()).toBe("2 of 3 in New"));
    const next = screen.getByRole("button", { name: "Next" }) as HTMLButtonElement;
    expect(next.disabled).toBe(false);
    fireEvent.click(next);
    await screen.findByText("Message fb_0");
    expect(calls.some((c) => c.method === "GET" && c.path.includes("cursor=c1"))).toBe(true);
    await waitFor(() => expect(position()).toBe("3 of 3 in New"));
  });

  it("a deep link shows no position and Back goes to the inbox", async () => {
    mockApi(server(rows()).handler);
    renderRoutes([{ pathname: "/feedback/fb_2" }], pages);
    await screen.findByText("Message fb_2");
    expect(position()).toBeUndefined();
    expect(screen.queryByRole("button", { name: "Previous" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Next" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Back to Inbox" }));
    await screen.findByRole("heading", { name: "Inbox page" });
  });

  it("auto-advances after a decision that removes the item and offers Undo", async () => {
    const srv = server(rows());
    const { calls } = mockApi(srv.handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.click(screen.getByRole("button", { name: "Mark reviewed" }));
    await screen.findByText("Message fb_1");
    expect(calls.find((c) => c.method === "PATCH")?.path).toBe("/api/feedback/fb_2");
    expect(position()).toBe("2 of 2 in New");
    expect(screen.getByText("Marked 1 reviewed")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Undo" })).toBeTruthy();
    await waitFor(() => expect(screen.getByTestId("announcer").textContent).toBe("Marked as reviewed. Press z to undo."));
  });

  it("Undo after auto-advance returns to the restored item", async () => {
    const srv = server(rows());
    const { calls } = mockApi(srv.handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.keyDown(document.body, { key: "e" });
    await screen.findByText("Message fb_1");
    fireEvent.keyDown(document.body, { key: "z" });
    await screen.findByText("Message fb_2");
    expect(calls.find((c) => c.path === "/api/feedback/status")?.body).toEqual({ ids: ["fb_2"], status: "new" });
    await waitFor(() => expect(position()).toBe("2 of 3 in New"));
    expect(srv.statusOf("fb_2")).toBe("new");
    expect(labelOf(document.querySelector(".status-actions button:disabled"))).toBe("Mark new");
  });

  it("with advance off, a decision stays on the item", async () => {
    window.localStorage.setItem("fbx.pref.advance", "0");
    const srv = server(rows());
    const { calls } = mockApi(srv.handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.click(screen.getByRole("button", { name: "Mark reviewed" }));
    await screen.findByText("Marked 1 reviewed");
    expect(calls.find((c) => c.method === "PATCH")?.path).toBe("/api/feedback/fb_2");
    // Still on fb_2, refetched with its new status; Undo is offered.
    await waitFor(() => expect(labelOf(document.querySelector(".status-actions button:disabled"))).toBe("Mark reviewed"));
    expect(screen.getByText("Message fb_2")).toBeTruthy();
    expect(screen.queryByText("Message fb_1")).toBeNull();
    expect(calls.filter((c) => c.method === "GET" && c.path === "/api/feedback/fb_2").length).toBeGreaterThanOrEqual(2);
    expect(screen.getByRole("button", { name: "Undo" })).toBeTruthy();
    // Undo restores in place and does not navigate.
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(srv.statusOf("fb_2")).toBe("new"));
    expect(screen.getByText("Message fb_2")).toBeTruthy();
  });

  it("at the end of the list a decision returns to the list", async () => {
    mockApi(server(rows()).handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_1", state: { from } }], pages);
    await screen.findByText("Message fb_1");
    fireEvent.click(screen.getByRole("button", { name: "Mark reviewed" }));
    await screen.findByRole("heading", { name: "Inbox page" });
    await screen.findByRole("link", { name: "Message fb_2" });
    expect(screen.queryByRole("link", { name: "Message fb_1" })).toBeNull();
    // Focus goes to the row before the one that left.
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("link", { name: "Message fb_2" })));
    expect(screen.getByRole("button", { name: "Undo" })).toBeTruthy();
  });

  it("Escape returns to the list with that row focused", async () => {
    mockApi(server(rows()).handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.keyDown(document.body, { key: "Escape" });
    await screen.findByRole("heading", { name: "Inbox page" });
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("link", { name: "Message fb_2" })));
    expect(screen.getAllByRole("link").map((a) => a.textContent)).toEqual(["Message fb_3", "Message fb_2", "Message fb_1"]);
  });

  it("Back from a detail opened on the per-user page returns to the per-user page", async () => {
    mockApi(server(rows()).handler);
    const userFilters: Filters = { ...newFilters, status: "all", userId: "cust-1" };
    const from: FromList = { kind: "user", key: listKey(userFilters), filters: userFilters, path: "/user/cust-1", label: "cust-1" };
    openOn(rows(), { from });
    renderRoutes([{ pathname: "/user/cust-1" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    expect(position()).toBe("2 of 3 from cust-1");
    const back = screen.getByRole("button", { name: "Back to feedback from cust-1" });
    expect(back.textContent).toBe("cust-1");
    fireEvent.click(back);
    await screen.findByRole("heading", { name: "User page" });
    expect(screen.queryByRole("heading", { name: "Inbox page" })).toBeNull();
    cleanup();
    // Opened directly with the origin in state: Back still goes to the user's page.
    renderRoutes([{ pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.click(screen.getByRole("button", { name: "Back to feedback from cust-1" }));
    await screen.findByRole("heading", { name: "User page" });
  });

  it("delete returns to the originating list", async () => {
    const srv = server(rows());
    const { calls } = mockApi(srv.handler);
    const from = openOn(rows());
    renderRoutes([{ pathname: "/" }, { pathname: "/feedback/fb_2", state: { from } }], pages);
    await screen.findByText("Message fb_2");
    fireEvent.click(screen.getByRole("button", { name: "Delete feedback" }));
    const dialog = await screen.findByRole("alertdialog");
    await act(async () => {
      fireEvent.click(within(dialog).getByRole("button", { name: "Delete feedback" }));
    });
    await screen.findByRole("heading", { name: "Inbox page" });
    expect(calls.some((c) => c.method === "DELETE" && c.path === "/api/feedback/fb_2")).toBe(true);
    await screen.findByRole("link", { name: "Message fb_3" });
    expect(screen.queryByRole("link", { name: "Message fb_2" })).toBeNull();
    // Focus lands on the row that followed the deleted one.
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("link", { name: "Message fb_1" })));
    expect(screen.getByText("Feedback deleted.")).toBeTruthy();
  });
});
