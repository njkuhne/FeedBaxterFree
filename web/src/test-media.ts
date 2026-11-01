import { NARROW_QUERY } from "./lib/useMediaQuery";

/**
 * Installs a `window.matchMedia` stub that matches NARROW_QUERY when `narrow` is true and nothing
 * else. Returns a function that removes it again (call it in afterEach).
 */
export function mockMatchMedia(narrow: boolean): () => void {
  const had = Object.prototype.hasOwnProperty.call(window, "matchMedia");
  const previous = window.matchMedia as unknown;
  const stub = (query: string): MediaQueryList =>
    ({
      matches: narrow && query === NARROW_QUERY,
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList;
  Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: stub });
  return () => {
    if (had) Object.defineProperty(window, "matchMedia", { configurable: true, writable: true, value: previous });
    else delete (window as unknown as Record<string, unknown>)["matchMedia"];
  };
}
