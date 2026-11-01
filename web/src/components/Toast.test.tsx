import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, useToast } from "./Toast";

const log: string[] = [];

function Trigger() {
  const toast = useToast();
  return (
    <>
      <button onClick={() => toast.show("Channel Web created.", "success")}>ok</button>
      <button onClick={() => toast.show("Could not save.", "error")}>bad</button>
      <button onClick={() => toast.show("First", "success", { key: "k" })}>first</button>
      <button onClick={() => toast.show("Second", "success", { key: "k" })}>second</button>
      <button
        onClick={() =>
          toast.show("Marked 1 reviewed", "success", {
            key: "u",
            live: false,
            duration: 3000,
            action: { label: "Undo", onAction: () => log.push("action") },
            onClose: () => log.push("close"),
          })
        }
      >
        withAction
      </button>
    </>
  );
}

beforeEach(() => {
  vi.useFakeTimers();
  log.length = 0;
});
afterEach(() => vi.useRealTimers());

function setup() {
  render(
    <ToastProvider>
      <Trigger />
    </ToastProvider>,
  );
}

describe("Toast", () => {
  it("puts messages in the polite region", () => {
    setup();
    fireEvent.click(screen.getByText("ok"));
    const polite = screen.getByRole("status");
    expect(polite.textContent).toContain("Channel Web created.");
    expect(screen.getByRole("alert").textContent).not.toContain("Channel Web created.");
  });

  it("puts errors in the alert region", () => {
    setup();
    fireEvent.click(screen.getByText("bad"));
    expect(screen.getByRole("alert").textContent).toContain("Could not save.");
    expect(screen.getByRole("status").textContent).not.toContain("Could not save.");
  });

  it("dismisses on its own after 5 s (8 s for errors)", () => {
    setup();
    fireEvent.click(screen.getByText("ok"));
    fireEvent.click(screen.getByText("bad"));
    act(() => {
      vi.advanceTimersByTime(4900);
    });
    expect(screen.queryByText("Channel Web created.")).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(screen.queryByText("Channel Web created.")).toBeNull();
    expect(screen.queryByText("Could not save.")).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(3000);
    });
    expect(screen.queryByText("Could not save.")).toBeNull();
  });

  it("pauses while hovered and can be dismissed by hand", () => {
    setup();
    fireEvent.click(screen.getByText("ok"));
    const toast = screen.getByText("Channel Web created.").closest(".toast");
    if (!toast) throw new Error("no toast");
    fireEvent.mouseEnter(toast);
    act(() => {
      vi.advanceTimersByTime(20000);
    });
    expect(screen.queryByText("Channel Web created.")).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));
    expect(screen.queryByText("Channel Web created.")).toBeNull();
  });

  it("keeps at most three toasts", () => {
    setup();
    for (let i = 0; i < 5; i++) fireEvent.click(screen.getByText("ok"));
    expect(screen.getAllByRole("button", { name: "Dismiss" })).toHaveLength(3);
  });
});

describe("Toast options", () => {
  it("a keyed toast replaces the previous one", () => {
    setup();
    fireEvent.click(screen.getByText("first"));
    expect(screen.queryByText("First")).not.toBeNull();
    fireEvent.click(screen.getByText("second"));
    expect(screen.queryByText("First")).toBeNull();
    expect(screen.queryByText("Second")).not.toBeNull();
    expect(screen.getAllByRole("button", { name: "Dismiss" })).toHaveLength(1);
  });

  it("the action button runs and closes the toast", () => {
    setup();
    fireEvent.click(screen.getByText("withAction"));
    // Not in a live region: the caller announces it.
    expect(screen.getByRole("status").textContent).not.toContain("Marked 1 reviewed");
    fireEvent.click(screen.getByRole("button", { name: "Undo" }));
    expect(log).toEqual(["action", "close"]);
    expect(screen.queryByText("Marked 1 reviewed")).toBeNull();
  });

  it("pauses on hover and focus and closes after the duration", () => {
    setup();
    fireEvent.click(screen.getByText("withAction"));
    const toast = screen.getByText("Marked 1 reviewed").closest(".toast");
    if (!toast) throw new Error("no toast");
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    fireEvent.mouseEnter(toast);
    act(() => {
      vi.advanceTimersByTime(20000);
    });
    expect(screen.queryByText("Marked 1 reviewed")).not.toBeNull();
    fireEvent.mouseLeave(toast);
    fireEvent.focus(screen.getByRole("button", { name: "Undo" }));
    act(() => {
      vi.advanceTimersByTime(20000);
    });
    expect(screen.queryByText("Marked 1 reviewed")).not.toBeNull();
    fireEvent.blur(screen.getByRole("button", { name: "Undo" }));
    act(() => {
      vi.advanceTimersByTime(900);
    });
    expect(screen.queryByText("Marked 1 reviewed")).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(screen.queryByText("Marked 1 reviewed")).toBeNull();
    expect(log).toEqual(["close"]);
  });
});
