import { screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fb, mockApi, renderPage } from "../feedback/testHelpers";
import type { FeedbackDetail as Detail, Role } from "../lib/types";
import FeedbackDetail from "./FeedbackDetail";

let role: Role = "admin";
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role, mustChangePassword: false } }),
}));

function detail(over: Partial<Detail> = {}): Detail {
  return { ...fb({ id: "fb_1" }), history: [], ...over };
}

function setup(d: Detail) {
  mockApi((c) => {
    if (c.path === "/api/feedback/fb_1") return { body: d };
    if (c.path === "/api/tags") return { body: { items: [] } };
    if (c.path === "/api/channels") return { body: { items: [{ id: "ch_1", name: "Default", createdAt: "x" }] } };
    return undefined;
  });
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
    first.unmount();
    const second = setup(detail({ status: "reviewed" }));
    expect(await screen.findByRole("button", { name: "Mark actioned" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Mark reviewed" })).toBeNull();
    second.unmount();
    setup(detail({ status: "actioned" }));
    await screen.findByRole("button", { name: "Mark as new" });
    expect(document.querySelectorAll(".btn-primary")).toHaveLength(0);
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
});
