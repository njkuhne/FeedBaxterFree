import { act, fireEvent, render, screen } from "@testing-library/react";
import { useRef, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";
import { Dialog } from "./Dialog";

function Harness({ onConfirm = () => Promise.resolve() }: { onConfirm?: () => Promise<void> }) {
  const [open, setOpen] = useState(false);
  return (
    <div id="app-root">
      <button onClick={() => setOpen(true)}>Open</button>
      {open ? (
        <ConfirmDialog
          danger
          title="Delete this feedback?"
          body="This permanently deletes the feedback."
          confirmLabel="Delete feedback"
          busyLabel="Deleting…"
          onConfirm={onConfirm}
          onClose={() => setOpen(false)}
        />
      ) : null}
    </div>
  );
}

function openDialog() {
  const opener = screen.getByRole("button", { name: "Open" });
  opener.focus();
  fireEvent.click(opener);
  return opener;
}

describe("Dialog", () => {
  it("opens with focus on Cancel and the right ARIA attributes", () => {
    render(<Harness />);
    openDialog();
    const dialog = screen.getByRole("alertdialog", { name: "Delete this feedback?" });
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.getAttribute("aria-labelledby")).toBeTruthy();
    expect(dialog.getAttribute("aria-describedby")).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Cancel" }));
    expect(document.getElementById("app-root")?.hasAttribute("inert")).toBe(true);
  });

  it("keeps Tab inside the dialog", () => {
    render(<Harness />);
    openDialog();
    const cancel = screen.getByRole("button", { name: "Cancel" });
    const confirm = screen.getByRole("button", { name: "Delete feedback" });
    expect(document.activeElement).toBe(cancel);
    fireEvent.keyDown(cancel, { key: "Tab" });
    // Tab from the first moves natively to the next; from the last it wraps.
    confirm.focus();
    fireEvent.keyDown(confirm, { key: "Tab" });
    expect(document.activeElement).toBe(cancel);
    fireEvent.keyDown(cancel, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(confirm);
  });

  it("closes on Escape and returns focus to the opener", () => {
    render(<Harness />);
    const opener = openDialog();
    fireEvent.keyDown(screen.getByRole("alertdialog"), { key: "Escape" });
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
    expect(document.getElementById("app-root")?.hasAttribute("inert")).toBe(false);
  });

  it("does not close on Escape while busy", async () => {
    let release: () => void = () => undefined;
    const pending = new Promise<void>((res) => {
      release = res;
    });
    render(<Harness onConfirm={() => pending} />);
    openDialog();
    fireEvent.click(screen.getByRole("button", { name: "Delete feedback" }));
    expect(screen.getByRole("button", { name: "Deleting…" }).getAttribute("aria-busy")).toBe("true");
    fireEvent.keyDown(screen.getByRole("alertdialog"), { key: "Escape" });
    expect(screen.queryByRole("alertdialog")).not.toBeNull();
    await act(async () => {
      release();
      await pending;
    });
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });

  it("shows an action error inside the dialog and stays open", async () => {
    render(<Harness onConfirm={() => Promise.reject(new Error("boom"))} />);
    openDialog();
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Delete feedback" }));
    });
    expect(screen.getByRole("alert").textContent).toBe("Something went wrong. Try again in a moment.");
    expect(screen.queryByRole("alertdialog")).not.toBeNull();
  });

  it("focuses the requested element and uses role dialog by default", () => {
    function Plain() {
      const field = useRef<HTMLInputElement | null>(null);
      return (
        <Dialog title="Reset" onClose={vi.fn()} initialFocusRef={field}>
          <button>First</button>
          <input aria-label="Field" ref={field} />
        </Dialog>
      );
    }
    render(<Plain />);
    expect(screen.getByRole("dialog", { name: "Reset" })).toBeTruthy();
    expect(document.activeElement).toBe(screen.getByLabelText("Field"));
  });
});
