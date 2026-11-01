import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, useToast } from "./Toast";

function Trigger() {
  const toast = useToast();
  return (
    <>
      <button onClick={() => toast.show("Channel Web created.", "success")}>ok</button>
      <button onClick={() => toast.show("Could not save.", "error")}>bad</button>
    </>
  );
}

beforeEach(() => vi.useFakeTimers());
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
