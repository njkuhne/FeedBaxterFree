import { act, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockApi, type Call } from "../feedback/testHelpers";
import { NewCountProvider } from "./NewCount";
import { Rail } from "./Rail";
import { usePageTitle } from "./usePageTitle";

vi.mock("./AuthProvider", () => ({
  useAuth: () => ({
    user: { id: "us_1", email: "nick@example.com", role: "admin", mustChangePassword: false },
    signOut: () => Promise.resolve(),
  }),
}));

function Page() {
  usePageTitle("Inbox");
  return <h1>Inbox page</h1>;
}

function withNew(n: number) {
  return mockApi((c: Call) =>
    c.path === "/api/stats" ? { body: { total: n, byStatus: n > 0 ? { new: n } : {}, byChannel: [], byTag: [] } } : undefined,
  );
}

const setup = () =>
  render(
    <MemoryRouter>
      <NewCountProvider>
        <Rail />
        <Page />
      </NewCountProvider>
    </MemoryRouter>,
  );

beforeEach(() => {
  vi.unstubAllGlobals();
  document.title = "";
});
afterEach(() => {
  vi.useRealTimers();
  Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
});

describe("NewCountProvider", () => {
  it("shows the New count next to Inbox in the rail", async () => {
    withNew(17);
    setup();
    const link = await screen.findByRole("link", { name: "Inbox, 17 new" });
    expect(link.textContent).toContain("Inbox");
    expect(link.querySelector(".nav-count")?.textContent).toBe("17");
    expect(link.querySelector(".nav-count")?.getAttribute("aria-hidden")).toBe("true");
  });

  it("puts the New count in the document title", async () => {
    withNew(17);
    setup();
    await screen.findByRole("link", { name: "Inbox, 17 new" });
    expect(document.title).toBe("(17) Inbox · FeedBaxter");
  });

  it("leaves the title alone when nothing is new", async () => {
    const { calls } = withNew(0);
    setup();
    await vi.waitFor(() => expect(calls.length).toBeGreaterThan(0));
    await act(async () => undefined);
    expect(document.title).toBe("Inbox · FeedBaxter");
    expect(screen.getByRole("link", { name: "Inbox" })).toBeTruthy();
    expect(document.querySelector(".nav-count")).toBeNull();
  });

  it("polls every 30 seconds only while visible", async () => {
    vi.useFakeTimers();
    const { calls } = withNew(1);
    setup();
    const n = () => calls.filter((c) => c.path === "/api/stats").length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(n()).toBe(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(n()).toBe(2);
    // Hidden: the timer stops.
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "hidden" });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(120_000);
    });
    expect(n()).toBe(2);
    // Visible again: one fetch at once.
    Object.defineProperty(document, "visibilityState", { configurable: true, value: "visible" });
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(n()).toBe(3);
  });
});
