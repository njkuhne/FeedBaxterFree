import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Status } from "../lib/types";
import { ListHeader } from "./ListHeader";
import { fb } from "./testHelpers";

type Props = Parameters<typeof ListHeader>[0];

function props(over: Partial<Props> = {}): Props {
  return {
    total: 17,
    loaded: 17,
    chips: [],
    onClearAll: () => undefined,
    status: "new" as Status | "all",
    allSelected: false,
    selectedItems: [],
    selectedCount: 0,
    busy: 0,
    tags: [{ id: "t1", name: "bug", count: 4 }],
    notice: null,
    onToggleAll: () => undefined,
    onStatus: () => undefined,
    onAddTag: () => Promise.resolve(true),
    onRemoveTag: () => undefined,
    onClear: () => undefined,
    ...over,
  };
}

describe("ListHeader", () => {
  it("shows removable filter chips and Clear all", () => {
    const removeTag = vi.fn();
    const clearAll = vi.fn();
    render(
      <ListHeader
        {...props({
          chips: [
            { key: "tag", label: "tag bug", onRemove: removeTag },
            { key: "q", label: "“safari”", onRemove: () => undefined },
          ],
          onClearAll: clearAll,
        })}
      />,
    );
    expect(screen.getByText("17 items")).toBeTruthy();
    expect(screen.getByText("tag bug")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Remove filter tag bug" }));
    expect(removeTag).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Remove filter “safari”" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear all" }));
    expect(clearAll).toHaveBeenCalledTimes(1);
  });

  it("shows a loaded count when the total is unknown and no Clear all without chips", () => {
    render(<ListHeader {...props({ total: null, loaded: 50 })} />);
    expect(screen.getByText("50 loaded")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Clear all" })).toBeNull();
  });

  it("the bulk bar replaces the header line in the same element", () => {
    const { container, rerender } = render(<ListHeader {...props()} />);
    const head = container.querySelector(".list-head") as HTMLElement;
    const className = head.className;
    expect(head.querySelector(".list-summary")).not.toBeNull();
    expect(within(head).queryByRole("toolbar")).toBeNull();

    rerender(<ListHeader {...props({ selectedCount: 2, selectedItems: [fb({ id: "a" }), fb({ id: "b" })] })} />);
    expect(container.querySelector(".list-head")).toBe(head);
    expect(head.className).toBe(className);
    expect(within(head).getByRole("toolbar", { name: "Bulk actions" })).toBeTruthy();
    expect(head.querySelector(".list-summary")).toBeNull();
    expect(within(head).getByText("2 selected")).toBeTruthy();
    // The select-all checkbox keeps the line's left edge.
    expect(head.firstElementChild?.querySelector("input[type=checkbox]")).not.toBeNull();

    rerender(<ListHeader {...props()} />);
    expect(container.querySelector(".list-head")).toBe(head);
    expect(head.querySelector(".list-summary")).not.toBeNull();
  });

  it("hides the action for the current tab", () => {
    const sel = { selectedCount: 1, selectedItems: [fb({ id: "a" })] };
    const { rerender } = render(<ListHeader {...props({ ...sel, status: "new" })} />);
    expect(screen.queryByRole("button", { name: "Mark new" })).toBeNull();
    expect(screen.getByRole("button", { name: "Mark reviewed" })).toBeTruthy();
    rerender(<ListHeader {...props({ ...sel, status: "reviewed" })} />);
    expect(screen.queryByRole("button", { name: "Mark reviewed" })).toBeNull();
    expect(screen.getByRole("button", { name: "Mark new" })).toBeTruthy();
    rerender(<ListHeader {...props({ ...sel, status: "all" })} />);
    for (const n of ["Mark reviewed", "Mark actioned", "Mark ignored", "Mark new"]) {
      expect(screen.getByRole("button", { name: n })).toBeTruthy();
    }
  });

  it("remove tag lists only tags on the selection", () => {
    const onRemoveTag = vi.fn();
    const selected = [fb({ id: "a", tags: ["bug", "export"] }), fb({ id: "b", tags: ["bug"] })];
    render(<ListHeader {...props({ selectedCount: 2, selectedItems: selected, tags: [{ id: "t1", name: "bug", count: 4 }, { id: "t2", name: "export", count: 2 }, { id: "t3", name: "other", count: 1 }], onRemoveTag })} />);
    fireEvent.click(screen.getByRole("button", { name: "Remove tag…" }));
    expect(screen.getByRole("button", { name: "bug (2)" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "export (1)" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /other/ })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "bug (2)" }));
    expect(onRemoveTag).toHaveBeenCalledWith("bug");
  });

  it("hides Remove tag when nothing selected carries a tag", () => {
    render(<ListHeader {...props({ selectedCount: 1, selectedItems: [fb({ id: "a" })] })} />);
    expect(screen.queryByRole("button", { name: "Remove tag…" })).toBeNull();
  });

  it("narrow header offers Select, then the checkbox and Done in Select mode", () => {
    const onSelectMode = vi.fn();
    const { rerender } = render(<ListHeader {...props({ narrow: true, selectMode: false, onSelectMode })} />);
    expect(screen.queryByLabelText("Select all on this page")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Select" }));
    expect(onSelectMode).toHaveBeenCalledWith(true);
    rerender(<ListHeader {...props({ narrow: true, selectMode: true, onSelectMode })} />);
    expect(screen.getByLabelText("Select all on this page")).toBeTruthy();
    expect(screen.getByText("Select items")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Done" }));
    expect(onSelectMode).toHaveBeenLastCalledWith(false);
  });
});
