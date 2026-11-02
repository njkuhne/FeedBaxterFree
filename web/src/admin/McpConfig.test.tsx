import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AnnouncerProvider } from "../components/Announcer";
import { ToastProvider } from "../components/Toast";
import * as api from "../lib/endpoints";
import Profile from "../pages/Profile";
import { McpConfig } from "./McpConfig";

vi.mock("../lib/endpoints");
vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({ user: { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false } }),
}));

beforeEach(() => vi.resetAllMocks());

describe("McpConfig", () => {
  it("fills the origin and token into both config forms", () => {
    render(
      <ToastProvider>
        <AnnouncerProvider>
          <McpConfig secret="fbx_pat_abc" origin="https://fb.example.com" />
        </AnnouncerProvider>
      </ToastProvider>,
    );
    expect(screen.getByLabelText("Claude Code").textContent).toBe(
      'claude mcp add --transport http feedbaxter https://fb.example.com/mcp --header "Authorization: Bearer fbx_pat_abc"',
    );
    const json = JSON.parse(screen.getByLabelText("JSON config (Claude Desktop and others)").textContent ?? "") as unknown;
    expect(json).toEqual({
      mcpServers: {
        feedbaxter: {
          type: "http",
          url: "https://fb.example.com/mcp",
          headers: { Authorization: "Bearer fbx_pat_abc" },
        },
      },
    });
    expect(screen.getByLabelText("JSON config (Claude Desktop and others)").textContent).toContain('\n  "mcpServers"');
    expect(screen.getByText("Paste one of these into your client. It includes the token, so keep it private.")).toBeTruthy();
  });

  it("disappears when the token is dismissed", async () => {
    vi.mocked(api.listTokens).mockResolvedValue({ items: [] });
    vi.mocked(api.createToken).mockResolvedValue({
      token: { id: "t1", name: "laptop", prefix: "fbx_pat_ab", createdAt: "2026-03-01T09:30:00.000Z", lastUsedAt: null },
      secret: "fbx_pat_secret1",
    });
    render(
      <MemoryRouter>
        <ToastProvider>
          <AnnouncerProvider>
            <Profile />
          </AnnouncerProvider>
        </ToastProvider>
      </MemoryRouter>,
    );
    fireEvent.change(await screen.findByLabelText("Token name"), { target: { value: "laptop" } });
    fireEvent.click(screen.getByRole("button", { name: "Create token" }));
    await waitFor(() => expect(screen.getByText("Connect your AI client")).toBeTruthy());
    expect(screen.getByLabelText("Claude Code").textContent).toContain("fbx_pat_secret1");
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText("Connect your AI client")).toBeNull();
    expect(screen.queryByText(/fbx_pat_secret1/)).toBeNull();
  });
});
