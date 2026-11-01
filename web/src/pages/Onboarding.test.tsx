import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mockApi, renderPage } from "../feedback/testHelpers";
import type { Role } from "../lib/types";
import Onboarding from "./Onboarding";

let role: Role = "admin";
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role, mustChangePassword: false } }),
}));

beforeEach(() => {
  vi.unstubAllGlobals();
  role = "admin";
});

describe("Onboarding", () => {
  it("lets an admin create a key and puts the secret in the curl command", async () => {
    const { calls } = mockApi((c) => {
      if (c.path === "/api/channels") return { body: { items: [{ id: "ch_9", name: "Default", createdAt: "x" }] } };
      if (c.path === "/api/channels/ch_9/keys") {
        return { status: 201, body: { key: { id: "ky_1" }, secret: "fbx_testsecret123" } };
      }
      return undefined;
    });
    renderPage(<Onboarding onReceived={() => undefined} />);
    expect(screen.getByRole("heading", { name: "Send your first feedback" })).toBeTruthy();
    expect(screen.getByLabelText("First API call").textContent).toContain("fbx_your_key");
    fireEvent.click(screen.getByRole("button", { name: "Create a key" }));
    await waitFor(() => expect(screen.getByLabelText("First API call").textContent).toContain("fbx_testsecret123"));
    expect(calls.some((c) => c.method === "POST" && c.path === "/api/channels/ch_9/keys")).toBe(true);
    expect(screen.queryByRole("button", { name: "Create a key" })).toBeNull();
    expect(screen.getByRole("region", { name: "Copy this key now" })).toBeTruthy();
  });

  it("tells a reviewer to ask an admin", () => {
    role = "reviewer";
    mockApi(() => undefined);
    renderPage(<Onboarding onReceived={() => undefined} />);
    expect(screen.getByText("Ask an admin for the ingest key.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Create a key" })).toBeNull();
  });

  it("switches over when the first feedback arrives", async () => {
    vi.useFakeTimers();
    const onReceived = vi.fn();
    mockApi((c) => (c.path === "/api/stats" ? { body: { total: 1, byStatus: { new: 1 }, byChannel: [], byTag: [] } } : undefined));
    renderPage(<Onboarding onReceived={onReceived} />);
    await vi.advanceTimersByTimeAsync(5100);
    vi.useRealTimers();
    expect(onReceived).toHaveBeenCalledTimes(1);
  });
});
