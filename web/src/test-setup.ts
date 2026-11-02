import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Node 25 ships its own `localStorage` global, which shadows jsdom's and is unusable without a file.
// Tests get a plain in-memory Storage instead, emptied after every test.
function memoryStorage(): Storage {
  const data = new Map<string, string>();
  return {
    get length() {
      return data.size;
    },
    clear: () => data.clear(),
    getItem: (k: string) => data.get(k) ?? null,
    key: (n: number) => Array.from(data.keys())[n] ?? null,
    removeItem: (k: string) => {
      data.delete(k);
    },
    setItem: (k: string, v: string) => {
      data.set(k, String(v));
    },
  };
}
Object.defineProperty(window, "localStorage", { configurable: true, writable: true, value: memoryStorage() });

afterEach(() => {
  cleanup();
  window.localStorage.clear();
});
