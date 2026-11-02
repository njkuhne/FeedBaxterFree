import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DEFAULT_PREFS, getPrefs, setPref, usePrefs } from "./prefs";

beforeEach(() => {
  // Reset any in-memory fallback left by a failed write.
  setPref("advance", true);
  setPref("singleKeys", true);
  window.localStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe("prefs", () => {
  it("defaults to advance on and single keys on", () => {
    expect(getPrefs()).toEqual({ advance: true, singleKeys: true });
    expect(DEFAULT_PREFS).toEqual({ advance: true, singleKeys: true });
    window.localStorage.setItem("fbx.pref.advance", "maybe");
    expect(getPrefs().advance).toBe(true);
  });

  it("persists a switch and reads it back", () => {
    setPref("advance", false);
    expect(window.localStorage.getItem("fbx.pref.advance")).toBe("0");
    expect(getPrefs()).toEqual({ advance: false, singleKeys: true });
    setPref("singleKeys", false);
    expect(window.localStorage.getItem("fbx.pref.singleKeys")).toBe("0");
    setPref("advance", true);
    expect(window.localStorage.getItem("fbx.pref.advance")).toBe("1");
    expect(getPrefs()).toEqual({ advance: true, singleKeys: false });
  });

  it("falls back to defaults when storage throws", () => {
    vi.spyOn(window.localStorage, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(window.localStorage, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(getPrefs()).toEqual({ advance: true, singleKeys: true });
    // A failed write still holds for this tab.
    setPref("singleKeys", false);
    expect(getPrefs().singleKeys).toBe(false);
    vi.restoreAllMocks();
    setPref("singleKeys", true);
    expect(getPrefs().singleKeys).toBe(true);
  });

  it("notifies subscribers when a pref changes", () => {
    const { result } = renderHook(() => usePrefs());
    expect(result.current.advance).toBe(true);
    act(() => setPref("advance", false));
    expect(result.current.advance).toBe(false);
    window.localStorage.setItem("fbx.pref.advance", "1");
    act(() => {
      window.dispatchEvent(new StorageEvent("storage", { key: "fbx.pref.advance" }));
    });
    expect(result.current.advance).toBe(true);
  });
});
