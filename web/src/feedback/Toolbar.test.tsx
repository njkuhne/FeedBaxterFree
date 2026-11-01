import { fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Filters } from "../lib/types";
import { Toolbar } from "./Toolbar";

const base: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };
const channels = [
  { id: "ch_1", name: "Web", createdAt: "2026-01-01T00:00:00Z" },
  { id: "ch_2", name: "iOS app", createdAt: "2026-01-01T00:00:00Z" },
];
const tags = [{ id: "t1", name: "bug", count: 12 }];

function setup(over: { narrow?: boolean; filters?: Filters; search?: string; count?: number | null; onSearch?: (q: string) => void; onClearSearch?: () => void } = {}) {
  const searchRef = createRef<HTMLInputElement>();
  const utils = render(
    <Toolbar
      filters={over.filters ?? base}
      search={over.search ?? ""}
      searchRef={searchRef}
      tags={tags}
      channels={channels}
      channelCounts={{ ch_1: 23, ch_2: 4 }}
      count={over.count === undefined ? 17 : over.count}
      onSearch={over.onSearch ?? (() => undefined)}
      onClearSearch={over.onClearSearch ?? (() => undefined)}
      onTag={() => undefined}
      onChannel={() => undefined}
      {...(over.narrow ? { narrow: true } : {})}
    />,
  );
  return { ...utils, searchRef };
}

describe("Toolbar", () => {
  it("renders search, tag, channel and export on one toolbar line", () => {
    const { container } = setup();
    const bars = container.querySelectorAll(".toolbar");
    expect(bars).toHaveLength(1);
    const bar = bars[0] as HTMLElement;
    const search = screen.getByLabelText("Search");
    const tag = screen.getByLabelText("Tag");
    const channel = screen.getByLabelText("Channel");
    const exportButton = screen.getByRole("button", { name: "Export" });
    for (const el of [search, tag, channel, exportButton]) expect(bar.contains(el)).toBe(true);
    // Labels exist but are visually hidden.
    for (const name of ["Search", "Tag", "Channel"]) {
      const label = Array.from(container.querySelectorAll("label")).find((l) => l.textContent === name);
      expect(label?.className).toBe("visually-hidden");
    }
    expect(screen.getByRole("option", { name: "bug (12)" })).toBeTruthy();
    expect(screen.getByRole("option", { name: "Web (23)" })).toBeTruthy();
  });

  it("the clear button empties the search", () => {
    const onClearSearch = vi.fn();
    const first = setup({ search: "" });
    expect(screen.queryByRole("button", { name: "Clear search" })).toBeNull();
    first.unmount();
    const { searchRef } = setup({ search: "safari", onClearSearch });
    fireEvent.click(screen.getByRole("button", { name: "Clear search" }));
    expect(onClearSearch).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(searchRef.current);
  });

  it("export says what it will export", () => {
    setup({ filters: { ...base, tag: "bug", q: "safari" }, count: 17 });
    const button = screen.getByRole("button", { name: "Export" });
    expect(button.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(button);
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText('Export 17 items · New, tag bug, matching "safari"')).toBeTruthy();
    const csv = screen.getByRole("link", { name: "CSV" });
    expect(csv.getAttribute("href")).toContain("format=csv");
    expect(csv.getAttribute("href")).toContain("tag=bug");
    fireEvent.keyDown(button, { key: "Escape" });
    expect(button.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(button);
  });

  it("export says all matching while the count is unknown", () => {
    setup({ count: null, filters: { ...base, status: "all", channel: "ch_2" } });
    fireEvent.click(screen.getByRole("button", { name: "Export" }));
    expect(screen.getByText("Export all matching items · channel iOS app")).toBeTruthy();
  });

  it("narrow toolbar shows search and a Filters button, not the selects", () => {
    setup({ narrow: true });
    expect(screen.getByLabelText("Search")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Filters" })).toBeTruthy();
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByRole("button", { name: "Export" })).toBeNull();
    expect((document.querySelector(".filters-panel") as HTMLElement).hidden).toBe(true);
  });

  it("Filters button shows the active count", () => {
    setup({ narrow: true, filters: { ...base, tag: "bug", channel: "ch_2", q: "safari" } });
    expect(screen.getByRole("button", { name: "Filters \u00b7 2" })).toBeTruthy();
  });

  it("Filters button toggles the disclosure with aria-expanded", () => {
    setup({ narrow: true });
    const button = screen.getByRole("button", { name: "Filters" });
    const panel = document.querySelector(".filters-panel") as HTMLElement;
    expect(button.getAttribute("aria-expanded")).toBe("false");
    expect(button.getAttribute("aria-controls")).toBe(panel.id);
    fireEvent.click(button);
    expect(button.getAttribute("aria-expanded")).toBe("true");
    expect(panel.hidden).toBe(false);
    fireEvent.keyDown(screen.getByLabelText("Tag"), { key: "Escape" });
    expect(button.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(button);
    fireEvent.click(button);
    fireEvent.click(button);
    expect(panel.hidden).toBe(true);
  });

  it("the disclosure holds Tag, Channel and Export", () => {
    setup({ narrow: true, filters: { ...base, tag: "bug" }, count: 17 });
    fireEvent.click(screen.getByRole("button", { name: "Filters \u00b7 1" }));
    const panel = document.querySelector(".filters-panel") as HTMLElement;
    for (const label of ["Tag", "Channel"]) {
      const el = screen.getByLabelText(label);
      expect(panel.contains(el)).toBe(true);
      const l = Array.from(panel.querySelectorAll("label")).find((x) => x.textContent === label);
      expect(l?.className).toBe("field-label");
    }
    expect(panel.textContent).toContain("Export 17 items \u00b7 New, tag bug");
    expect(screen.getByRole("link", { name: "CSV" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "JSON" })).toBeTruthy();
  });

  it("desktop toolbar is unchanged", () => {
    setup();
    expect(screen.getByLabelText("Tag")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Export" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /^Filters/ })).toBeNull();
    expect(document.querySelector(".filters-panel")).toBeNull();
  });
});
