import { fireEvent, render, screen, waitFor } from "@testing-library/react";
// @ts-expect-error The project has no Node type definitions (see vite.config.ts); Vitest runs on Node.
import { readFileSync } from "node:fs";
import { describe, expect, it, vi } from "vitest";
import type { TagCount } from "../lib/types";
import { TAG_ERROR } from "./statusCopy";
import { TagEditor } from "./TagEditor";

// Node global, declared as vite.config.ts does.
declare const process: { cwd: () => string };

const suggestions: TagCount[] = [
  { id: "t1", name: "ui", count: 3 },
  { id: "t2", name: "bug", count: 12 },
  { id: "t3", name: "export", count: 7 },
];

function setup(over: Partial<Parameters<typeof TagEditor>[0]> = {}) {
  const onAdd = vi.fn((name: string) => {
    void name;
    return Promise.resolve(true);
  });
  const utils = render(<TagEditor tags={[]} suggestions={suggestions} onAdd={onAdd} {...over} />);
  return { onAdd, ...utils };
}

const openEditor = () => {
  fireEvent.click(screen.getByRole("button", { name: "+ Add tag" }));
  return screen.getByRole("combobox") as HTMLInputElement;
};

describe("TagEditor", () => {
  it("adds a tag with Enter and keeps the input open", async () => {
    const { onAdd } = setup();
    const input = openEditor();
    fireEvent.change(input, { target: { value: "  regression " } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith("regression"));
    await waitFor(() => expect(input.value).toBe(""));
    expect(screen.getByRole("combobox")).toBe(input);
    expect(document.activeElement).toBe(input);
  });

  it("arrow keys choose a suggestion sorted by use", async () => {
    const { onAdd } = setup();
    const input = openEditor();
    expect(screen.getAllByRole("option").map((o) => o.getAttribute("aria-label"))).toEqual([
      "bug, 12 items",
      "export, 7 items",
      "ui, 3 items",
    ]);
    expect(input.getAttribute("aria-activedescendant")).toBeNull();
    fireEvent.keyDown(input, { key: "ArrowDown" });
    const [first, second] = screen.getAllByRole("option");
    expect(input.getAttribute("aria-activedescendant")).toBe(first?.id);
    expect(first?.getAttribute("aria-selected")).toBe("true");
    fireEvent.keyDown(input, { key: "ArrowDown" });
    expect(input.getAttribute("aria-activedescendant")).toBe(second?.id);
    fireEvent.keyDown(input, { key: "ArrowUp" });
    fireEvent.keyDown(input, { key: "ArrowUp" });
    // Up from the first wraps to the last.
    expect(input.getAttribute("aria-activedescendant")).toBe(screen.getAllByRole("option")[2]?.id);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "ArrowDown" });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith("export"));
  });

  it("filters suggestions by the typed text and leaves out tags the item has", () => {
    setup({ tags: ["bug"] });
    const input = openEditor();
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["export7", "ui3"]);
    fireEvent.change(input, { target: { value: "UI" } });
    expect(screen.getAllByRole("option").map((o) => o.textContent)).toEqual(["ui3"]);
  });

  it("Escape closes the input and returns focus to Add tag", async () => {
    setup();
    const input = openEditor();
    expect(input.getAttribute("aria-expanded")).toBe("true");
    // The first Escape closes the suggestions only.
    fireEvent.keyDown(input, { key: "Escape" });
    expect(input.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByRole("combobox")).toBe(input);
    fireEvent.keyDown(input, { key: "Escape" });
    expect(screen.queryByRole("combobox")).toBeNull();
    const add = screen.getByRole("button", { name: "+ Add tag" });
    await waitFor(() => expect(document.activeElement).toBe(add));
  });

  it("rejects an empty or too long tag", async () => {
    const { onAdd } = setup();
    const input = openEditor();
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByText(TAG_ERROR)).toBeTruthy();
    expect(input.getAttribute("aria-invalid")).toBe("true");
    fireEvent.change(input, { target: { value: "x" } });
    expect(screen.queryByText(TAG_ERROR)).toBeNull();
    fireEvent.change(input, { target: { value: "y".repeat(65) } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByText(TAG_ERROR)).toBeTruthy();
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByText(TAG_ERROR)).toBeTruthy();
    expect(onAdd).not.toHaveBeenCalled();
  });

  it("clears the input without adding a tag the item already has", () => {
    const { onAdd } = setup({ tags: ["Bug"] });
    const input = openEditor();
    fireEvent.change(input, { target: { value: "bug" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(input.value).toBe("");
    expect(onAdd).not.toHaveBeenCalled();
  });

  it("adds a clicked suggestion", async () => {
    const { onAdd } = setup();
    openEditor();
    fireEvent.click(screen.getByRole("option", { name: "ui, 3 items" }));
    await waitFor(() => expect(onAdd).toHaveBeenCalledWith("ui"));
  });

  it("remove buttons are labelled and at least 44 px", () => {
    const onRemove = vi.fn();
    setup({ tags: ["bug"], onRemove });
    const button = screen.getByRole("button", { name: "Remove tag bug" });
    fireEvent.click(button);
    expect(onRemove).toHaveBeenCalledWith("bug");
    // The visible button is 32 px; its ::before carries the 44 px hit area.
    expect(button.className).toBe("pill-remove");
    const css = String(readFileSync(`${process.cwd()}/src/styles/components.css`, "utf8"));
    const block = (selector: string) => {
      const at = css.indexOf(`${selector} {`);
      return at < 0 ? "" : css.slice(at, css.indexOf("}", at));
    };
    expect(block(".pill-remove")).toMatch(/width: 32px;/);
    expect(block(".pill-remove")).toMatch(/height: 32px;/);
    expect(block(".pill-remove::before")).toMatch(/width: var\(--target\);/);
    expect(block(".pill-remove::before")).toMatch(/height: var\(--target\);/);
  });
});
