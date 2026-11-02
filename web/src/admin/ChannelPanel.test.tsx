import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as api from "../lib/endpoints";
import type { Channel, Key } from "../lib/types";
import { ChannelPanel, deleteBody } from "./ChannelPanel";

vi.mock("../lib/endpoints");

const channel: Channel = { id: "ch_1", name: "Web", createdAt: "2026-03-01T09:30:00.000Z" };
const key = (id: string, revokedAt: string | null): Key => ({
  id,
  channelId: "ch_1",
  prefix: `fbx_${id}`,
  createdAt: "2026-03-01T09:30:00.000Z",
  revokedAt,
});

function setup(keys: Key[], channelCount = 2, onKeyCreated = vi.fn(), ch: Channel = channel, itemCount: number | null = null) {
  render(
    <div id="app-root">
      <ChannelPanel
        channel={ch}
        itemCount={itemCount}
        keys={keys}
        channelCount={channelCount}
        onRenamed={vi.fn()}
        onDeleted={vi.fn()}
        onKeyCreated={onKeyCreated}
        onKeyRevoked={vi.fn()}
      />
    </div>,
  );
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe("ChannelPanel", () => {
  it.each([
    [0, "It has no active keys. Feedback already received is kept."],
    [1, "Its 1 active key is revoked immediately, so apps using them will start failing. Feedback already received is kept."],
    [2, "Its 2 active keys are revoked immediately, so apps using them will start failing. Feedback already received is kept."],
  ])("delete dialog body for %i active keys", (n, text) => {
    expect(deleteBody(n)).toBe(text);
    const keys = Array.from({ length: n }, (_, i) => key(`a${i}`, null)).concat([key("old", "2026-03-02T12:00:00.000Z")]);
    setup(keys);
    fireEvent.click(screen.getByRole("button", { name: "Delete channel" }));
    expect(screen.getByRole("alertdialog", { name: "Delete channel Web?" })).toBeTruthy();
    expect(screen.getByText(text)).toBeTruthy();
  });

  it("explains that the only channel cannot be deleted", () => {
    setup([], 1);
    expect(screen.getByText("The only channel cannot be deleted.")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Delete channel" })).toBeNull();
  });

  it("offers Revoke only on active keys and labels the table", () => {
    setup([key("a1", null), key("r1", "2026-03-02T12:00:00.000Z")]);
    expect(screen.getAllByRole("button", { name: /^Revoke key/ })).toHaveLength(1);
    expect(screen.getByRole("table", { name: "Keys for Web" })).toBeTruthy();
    expect(screen.getByText("Revoked 2 Mar 2026")).toBeTruthy();
    expect(screen.getByText("Active")).toBeTruthy();
  });

  it("shows a created secret once, in the panel", async () => {
    const created = key("new1", null);
    vi.mocked(api.createKey).mockResolvedValue({ key: created, secret: "fbx_supersecret" });
    const onKeyCreated = vi.fn();
    setup([], 2, onKeyCreated);
    expect(screen.getByText("No keys yet.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Create key" }));
    await waitFor(() => expect(screen.getByRole("region", { name: "Copy this key now" })).toBeTruthy());
    expect((screen.getByLabelText("Secret value") as HTMLInputElement).value).toBe("fbx_supersecret");
    expect(screen.getByText("It is shown only once. Only a hash is stored.")).toBeTruthy();
    expect(onKeyCreated).toHaveBeenCalledWith(created);
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByLabelText("Secret value")).toBeNull();
    expect(screen.queryByText("fbx_supersecret")).toBeNull();
  });

  it("shows the item count and last received time", () => {
    const recent = new Date(Date.now() - 2 * 3600 * 1000).toISOString();
    setup([], 2, vi.fn(), { ...channel, lastReceivedAt: recent }, 312);
    expect(screen.getByText("312 items · last received 2 h ago")).toBeTruthy();
    expect(screen.getByText("created 1 Mar 2026")).toBeTruthy();
  });

  it("shows No feedback yet for an empty channel", () => {
    setup([], 2, vi.fn(), { ...channel, lastReceivedAt: null }, 0);
    expect(screen.getByText("No feedback yet")).toBeTruthy();
  });

  it("key table shows last used or Never", () => {
    const used = new Date(Date.now() - 5 * 60 * 1000).toISOString();
    setup([{ ...key("a1", null), lastUsedAt: used }, key("a2", null)]);
    expect(screen.getByRole("columnheader", { name: "Last used" })).toBeTruthy();
    const cell = screen.getByText("5 min ago");
    expect(cell.getAttribute("title")).toMatch(/^\d+ \w{3} \d{4}, \d{2}:\d{2}$/);
    expect(screen.getByText("Never")).toBeTruthy();
  });

  it("Delete channel is separate from Rename", () => {
    setup([key("a1", null)]);
    const del = screen.getByRole("button", { name: "Delete channel" });
    const rename = screen.getByRole("button", { name: "Rename" });
    expect(del.closest(".channel-danger")).not.toBeNull();
    expect(del.closest(".channel-head")).toBeNull();
    expect(rename.closest(".channel-head")).not.toBeNull();
    expect(rename.compareDocumentPosition(del) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(document.querySelector(".channel-danger")?.previousElementSibling?.querySelector("table, .table-scroll")).not.toBeNull();
  });
});
