import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mockApi, renderPage, type Call } from "../feedback/testHelpers";
import Channels from "./Channels";

vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false } }),
}));

beforeEach(() => vi.unstubAllGlobals());

const serve = () =>
  mockApi((c: Call) => {
    if (c.path === "/api/channels" && c.method === "GET") {
      return { body: { items: [{ id: "ch_1", name: "Web", createdAt: "2026-03-01T09:30:00.000Z", lastReceivedAt: null }] } };
    }
    if (c.path === "/api/keys") return { body: { items: [] } };
    if (c.path === "/api/stats") return { body: { total: 0, byStatus: {}, byChannel: [], byTag: [] } };
    return undefined;
  });

describe("Channels page", () => {
  it("New channel reveals the name field", async () => {
    serve();
    renderPage(<Channels />);
    const button = await screen.findByRole("button", { name: "New channel" });
    expect(screen.queryByLabelText("New channel name")).toBeNull();
    expect(document.querySelector(".btn-primary")).toBeNull();
    fireEvent.click(button);
    const field = screen.getByLabelText("New channel name");
    expect(document.activeElement).toBe(field);
    expect(screen.getByRole("button", { name: "Create" })).toBeTruthy();
    fireEvent.keyDown(field, { key: "Escape" });
    expect(screen.queryByLabelText("New channel name")).toBeNull();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole("button", { name: "New channel" })));
  });
});
