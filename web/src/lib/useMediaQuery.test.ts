import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { mockMatchMedia } from "../test-media";
import { useNarrow } from "./useMediaQuery";

describe("useMediaQuery", () => {
  it("useNarrow is false without matchMedia", () => {
    expect(typeof window.matchMedia).toBe("undefined");
    const { result } = renderHook(() => useNarrow());
    expect(result.current).toBe(false);
  });

  it("useNarrow follows the mocked media query", () => {
    const restore = mockMatchMedia(true);
    const narrow = renderHook(() => useNarrow());
    expect(narrow.result.current).toBe(true);
    restore();
    const restoreWide = mockMatchMedia(false);
    const wide = renderHook(() => useNarrow());
    expect(wide.result.current).toBe(false);
    restoreWide();
  });
});
