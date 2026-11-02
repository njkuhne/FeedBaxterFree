import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defaultFilters } from "../lib/filters";
import { ExportPanel } from "./ExportMenu";

beforeEach(() => {
  URL.createObjectURL = vi.fn(() => "blob:x");
  URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("ExportPanel", () => {
  it("export failure shows Export failed. Try again.", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("", { status: 500 }))));
    const onDone = vi.fn();
    render(<ExportPanel filters={defaultFilters} count={3} onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "CSV" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("Export failed. Try again.");
    expect(onDone).not.toHaveBeenCalled();
    expect((screen.getByRole("button", { name: "JSON" }) as HTMLButtonElement).disabled).toBe(false);
    // The next attempt clears the message.
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("a", { status: 200 }))));
    fireEvent.click(screen.getByRole("button", { name: "JSON" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("a successful export closes the panel", async () => {
    let release: (r: Response) => void = () => undefined;
    vi.stubGlobal("fetch", vi.fn(() => new Promise<Response>((r) => (release = r))));
    const onDone = vi.fn();
    render(<ExportPanel filters={defaultFilters} count={3} onDone={onDone} />);
    fireEvent.click(screen.getByRole("button", { name: "CSV" }));
    expect(screen.getByRole("button", { name: "Exporting…" })).toBeTruthy();
    expect((screen.getByRole("button", { name: "JSON" }) as HTMLButtonElement).disabled).toBe(true);
    release(new Response("a", { status: 200 }));
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1));
  });
});
