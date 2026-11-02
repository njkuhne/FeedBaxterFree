import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { useLocation } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { clearFeedbackCache } from "../feedback/useFeedbackList";
import { fb, mockApi, renderRoutes, type Call } from "../feedback/testHelpers";
import type { Feedback, Role } from "../lib/types";
import PerUser from "./PerUser";

let role: Role = "admin";
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role, mustChangePassword: false } }),
}));

function Probe() {
  const l = useLocation();
  return <output data-testid="loc">{l.pathname + l.search}</output>;
}

const mine: Feedback[] = [
  fb({ id: "fb_4", userId: "usr_1", message: "Four", status: "new", receivedAt: "2026-03-04T09:30:00.000Z", platform: "ios", appVersion: "3.1.0" }),
  fb({ id: "fb_3", userId: "usr_1", message: "Three", status: "reviewed", receivedAt: "2026-03-03T09:30:00.000Z", platform: "ios", appVersion: "3.1.0" }),
  fb({ id: "fb_2", userId: "usr_1", message: "Two", status: "new", receivedAt: "2026-03-02T09:30:00.000Z", platform: "ios", appVersion: "3.1.0" }),
  fb({ id: "fb_1", userId: "usr_1", message: "One", status: "ignored", receivedAt: "2026-03-01T09:30:00.000Z", platform: "ios", appVersion: "3.1.0" }),
];

function api(opts: { total?: number } = {}) {
  const total = opts.total ?? mine.length;
  return mockApi((c: Call) => {
    const url = new URL(c.path, "http://x");
    if (c.method === "GET" && url.pathname === "/api/feedback") {
      const status = url.searchParams.get("status");
      const items = mine.filter((i) => status === null || i.status === status).slice(0, total === 1 ? 1 : undefined);
      return { body: { items, nextCursor: null } };
    }
    if (c.method === "GET" && url.pathname === "/api/stats") {
      return { body: { total, byStatus: total === 1 ? { new: 1 } : { new: 2, reviewed: 1, ignored: 1 }, byChannel: [], byTag: [] } };
    }
    if (c.path === "/api/channels" || c.path === "/api/tags") return { body: { items: [] } };
    if (c.method === "POST" && c.path === "/api/feedback/delete-by-user") return { body: { deleted: total } };
    return undefined;
  });
}

function setup(at = "/user/usr_1") {
  return renderRoutes(
    [{ pathname: at }],
    [
      {
        path: "/user/:userId",
        element: (
          <>
            <PerUser />
            <Probe />
          </>
        ),
      },
    ],
  );
}

beforeEach(() => {
  clearFeedbackCache();
  vi.unstubAllGlobals();
  role = "admin";
});

describe("PerUser", () => {
  it("shows the summary line under the title", async () => {
    api();
    setup();
    const summary = await screen.findByText(/^4 items · first /);
    expect(summary.textContent).toMatch(/^4 items · first 1 Mar( 2026)? · latest 4 Mar( 2026)? · iOS 3\.1\.0$/);
    expect(summary.tagName).toBe("P");
    expect(summary.classList.contains("muted")).toBe(true);
    expect(summary.previousElementSibling?.classList.contains("page-head")).toBe(true);
  });

  it("status tabs filter the list and keep the status in the URL", async () => {
    const { calls } = api();
    setup();
    await screen.findByRole("link", { name: "Four" });
    const radios = screen.getAllByRole("radio").map((r) => r.closest("label")?.textContent);
    expect(radios).toEqual(["All 4", "New 2", "Reviewed 1", "Actioned 0", "Ignored 1"]);
    expect((screen.getByRole("radio", { name: "All 4" }) as HTMLInputElement).checked).toBe(true);
    fireEvent.click(screen.getByRole("radio", { name: "Reviewed 1" }));
    await screen.findByRole("link", { name: "Three" });
    await waitFor(() => expect(screen.queryByRole("link", { name: "Four" })).toBeNull());
    expect(screen.getByTestId("loc").textContent).toBe("/user/usr_1?status=reviewed");
    expect(calls.some((c) => c.path.includes("/api/feedback?") && c.path.includes("status=reviewed") && c.path.includes("userId=usr_1"))).toBe(true);
    // The summary is the user's, whatever the tab.
    expect(screen.getByText(/^4 items · /)).toBeTruthy();
    fireEvent.click(screen.getByRole("radio", { name: "All 4" }));
    await screen.findByRole("link", { name: "Four" });
    expect(screen.getByTestId("loc").textContent).toBe("/user/usr_1");
  });

  it("starts on the tab in the URL", async () => {
    api();
    setup("/user/usr_1?status=ignored");
    await screen.findByRole("link", { name: "One" });
    expect((screen.getByRole("radio", { name: "Ignored 1" }) as HTMLInputElement).checked).toBe(true);
  });

  it("delete all sits in its own section with a sentence", async () => {
    api();
    setup();
    const region = await screen.findByRole("region", { name: "Data removal" });
    expect(
      within(region).getByText(
        "Use this when your customer asks you to remove this person's data. It permanently deletes every item from usr_1.",
      ),
    ).toBeTruthy();
    expect(within(region).getByRole("button", { name: "Delete all feedback for this user" })).toBeTruthy();
  });

  it("hides the delete section from reviewers", async () => {
    role = "reviewer";
    api();
    setup();
    await screen.findByRole("link", { name: "Four" });
    expect(screen.queryByRole("region", { name: "Data removal" })).toBeNull();
  });

  it("the confirmation names the count", async () => {
    api();
    const first = setup();
    await screen.findByText(/^4 items · /);
    fireEvent.click(screen.getByRole("button", { name: "Delete all feedback for this user" }));
    expect(screen.getByRole("alertdialog", { name: "Delete all 4 feedback items from usr_1?" })).toBeTruthy();
    first.unmount();
    clearFeedbackCache();
    api({ total: 1 });
    setup();
    await screen.findByText(/^1 item · /);
    fireEvent.click(screen.getByRole("button", { name: "Delete all feedback for this user" }));
    expect(screen.getByRole("alertdialog", { name: "Delete the 1 feedback item from usr_1?" })).toBeTruthy();
  });

  it("after deletion the page says what happened and links back", async () => {
    const { calls } = api();
    setup();
    await screen.findByText(/^4 items · /);
    fireEvent.click(screen.getByRole("button", { name: "Delete all feedback for this user" }));
    fireEvent.click(within(screen.getByRole("alertdialog")).getByRole("button", { name: "Delete all feedback" }));
    const done = await screen.findByText("Deleted 4 feedback items from usr_1.");
    expect(calls.find((c) => c.path === "/api/feedback/delete-by-user")?.body).toEqual({ userId: "usr_1" });
    expect(done.getAttribute("role")).toBe("status");
    await waitFor(() => expect(document.activeElement).toBe(done));
    expect(screen.getByRole("link", { name: "Back to the inbox" }).getAttribute("href")).toBe("/");
    expect(screen.queryByRole("radio")).toBeNull();
    expect(screen.queryByRole("link", { name: "Four" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Data removal" })).toBeNull();
    expect(screen.queryByText("Feedback deleted.")).toBeNull();
    expect(screen.queryByText(/^Deleted 4 feedback items\.$/)).toBeNull();
  });
});
