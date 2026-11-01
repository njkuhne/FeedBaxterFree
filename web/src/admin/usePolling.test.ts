import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { usePolling } from "./usePolling";

function setVisibility(state: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => state });
  document.dispatchEvent(new Event("visibilitychange"));
}

beforeEach(() => {
  vi.useFakeTimers();
  setVisibility("visible");
});
afterEach(() => {
  vi.useRealTimers();
});

describe("usePolling", () => {
  it("runs immediately and then every interval", async () => {
    const fn = vi.fn(() => Promise.resolve());
    renderHook(() => usePolling(fn, 10_000));
    expect(fn).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(fn).toHaveBeenCalledTimes(2);
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("pauses while hidden and runs once when visible again", async () => {
    const fn = vi.fn(() => Promise.resolve());
    renderHook(() => usePolling(fn, 10_000));
    expect(fn).toHaveBeenCalledTimes(1);
    act(() => setVisibility("hidden"));
    await act(() => vi.advanceTimersByTimeAsync(60_000));
    expect(fn).toHaveBeenCalledTimes(1);
    await act(async () => setVisibility("visible"));
    expect(fn).toHaveBeenCalledTimes(2);
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(fn).toHaveBeenCalledTimes(3);
  });

  it("does not overlap a run that is still in flight", async () => {
    let release: () => void = () => undefined;
    const fn = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );
    const { result } = renderHook(() => usePolling(fn, 10_000));
    expect(fn).toHaveBeenCalledTimes(1);
    await act(() => vi.advanceTimersByTimeAsync(30_000));
    act(() => result.current.refresh());
    expect(fn).toHaveBeenCalledTimes(1);
    await act(async () => release());
    await act(() => vi.advanceTimersByTimeAsync(10_000));
    expect(fn).toHaveBeenCalledTimes(2);
  });

  it("refresh runs now and resets the timer, and unmount stops polling", async () => {
    const fn = vi.fn(() => Promise.resolve());
    const { result, unmount } = renderHook(() => usePolling(fn, 10_000));
    await act(() => vi.advanceTimersByTimeAsync(6_000));
    await act(async () => result.current.refresh());
    expect(fn).toHaveBeenCalledTimes(2);
    await act(() => vi.advanceTimersByTimeAsync(6_000));
    expect(fn).toHaveBeenCalledTimes(2);
    await act(() => vi.advanceTimersByTimeAsync(4_000));
    expect(fn).toHaveBeenCalledTimes(3);
    unmount();
    await act(() => vi.advanceTimersByTimeAsync(60_000));
    expect(fn).toHaveBeenCalledTimes(3);
  });
});
