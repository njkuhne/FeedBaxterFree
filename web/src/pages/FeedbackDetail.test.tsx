import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { listKey } from "../feedback/listStore";
import { fb, mockApi, renderPage, renderRoutes, type Call } from "../feedback/testHelpers";
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
    expect(document.querySelector(".btn-primary")?.textContent).toBe("Mark reviewed");
    first.unmount();
    const second = setup(detail({ status: "reviewed" }));
    await screen.findByRole("button", { name: "Mark actioned" });
    expect(document.querySelector(".btn-primary")?.textContent).toBe("Mark actioned");
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
      expect(buttons.map((b) => b.textContent)).toEqual(labels);
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
    await waitFor(() => expect(document.querySelector(".status-actions button:disabled")?.textContent).toBe("Mark actioned"));
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
    expect(await screen.findByText(/New → Reviewed · a@example.com · via MCP/)).toBeTruthy();
    expect(screen.getByText(/a removed user · via UI/)).toBeTruthy();
    expect(screen.getByText(/^Received · /)).toBeTruthy();
  });

  it("shows metadata values as text and objects as JSON", async () => {
    setup(detail({ metadata: { screen: "export", plan: { tier: "pro" } } }));
    await screen.findByText("export");
    expect(screen.getByText('{"tier":"pro"}')).toBeTruthy();
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
    await screen.findByText("Restored tag bug on 1 item.");
    expect(calls.find((c) => c.path === "/api/feedback/add-tags")?.body).toEqual({ ids: ["fb_1"], tags: ["bug"] });
    expect(await screen.findByRole("button", { name: "Remove tag bug" })).toBeTruthy();
  });
});
