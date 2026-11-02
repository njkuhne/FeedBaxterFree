import { useSyncExternalStore } from "react";

export interface Prefs {
  advance: boolean;
  singleKeys: boolean;
}

export const DEFAULT_PREFS: Prefs = { advance: true, singleKeys: true };

const STORAGE_KEY: Record<keyof Prefs, string> = {
  advance: "fbx.pref.advance",
  singleKeys: "fbx.pref.singleKeys",
};

/** Values whose write failed: kept for this tab so the switch still works without storage. */
const memory: Partial<Prefs> = {};
const listeners = new Set<() => void>();
let snapshot: Prefs = DEFAULT_PREFS;

function readOne(key: keyof Prefs): boolean {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY[key]);
    if (raw === "1") return true;
    if (raw === "0") return false;
  } catch {
    // storage missing or throwing: fall through to memory or the default
  }
  return memory[key] ?? DEFAULT_PREFS[key];
}

/** The current preferences, read at call time. A stable object while nothing changed. */
export function getPrefs(): Prefs {
  const next: Prefs = { advance: readOne("advance"), singleKeys: readOne("singleKeys") };
  if (next.advance !== snapshot.advance || next.singleKeys !== snapshot.singleKeys) snapshot = next;
  return snapshot;
}

export function setPref<K extends keyof Prefs>(key: K, value: Prefs[K]): void {
  try {
    window.localStorage.setItem(STORAGE_KEY[key], value ? "1" : "0");
    delete memory[key];
  } catch {
    memory[key] = value;
  }
  for (const fn of Array.from(listeners)) fn();
}

function subscribe(onChange: () => void): () => void {
  listeners.add(onChange);
  const onStorage = (e: StorageEvent) => {
    if (e.key === null || e.key === STORAGE_KEY.advance || e.key === STORAGE_KEY.singleKeys) onChange();
  };
  window.addEventListener("storage", onStorage);
  return () => {
    listeners.delete(onChange);
    window.removeEventListener("storage", onStorage);
  };
}

/** The preferences, live: every subscriber in this tab updates on `setPref`, other tabs on "storage". */
export function usePrefs(): Prefs {
  return useSyncExternalStore(subscribe, getPrefs, () => DEFAULT_PREFS);
}
