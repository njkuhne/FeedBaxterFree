import { useSyncExternalStore } from "react";

/** Below this width the shell is the compact top bar. CSS uses the same breakpoint. */
export const NARROW_QUERY = "(max-width: 879px)";

function hasMatchMedia(): boolean {
  return typeof window !== "undefined" && typeof window.matchMedia === "function";
}

/** Follows a media query. Without `window.matchMedia` (jsdom) it is false, so tests run as desktop. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    (onChange) => {
      if (!hasMatchMedia()) return () => undefined;
      const mql = window.matchMedia(query);
      mql.addEventListener("change", onChange);
      return () => mql.removeEventListener("change", onChange);
    },
    () => (hasMatchMedia() ? window.matchMedia(query).matches : false),
    () => false,
  );
}

export function useNarrow(): boolean {
  return useMediaQuery(NARROW_QUERY);
}
