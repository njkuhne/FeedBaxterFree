import { matchesFilters, toApiParams } from "../lib/filters";
import type { Feedback, FeedbackPage, Filters } from "../lib/types";

export interface CachedList {
  filters: Filters;
  items: Feedback[];
  nextCursor: string | null;
}

export type FeedbackEvent =
  | { type: "apply"; items: Feedback[]; reason: "change" | "undo" | "tags" }
  | { type: "remove"; ids: string[] }
  | { type: "page"; key: string };

// In-memory only: lists and the focused row per query, so Back from a detail page restores them
// and every status change, from the list or the detail page, reaches the same rows.
const lists = new Map<string, CachedList>();
const focus = new Map<string, string>();
const listeners = new Set<(e: FeedbackEvent) => void>();

function emit(e: FeedbackEvent): void {
  for (const fn of Array.from(listeners)) fn(e);
}

export function listKey(filters: Filters): string {
  return toApiParams(filters).toString();
}

export function getList(key: string): CachedList | undefined {
  return lists.get(key);
}

/** Stores a list without telling subscribers (the list hook writes its own state back). */
export function setList(key: string, list: CachedList): void {
  lists.set(key, list);
}

export function dropList(key: string): void {
  lists.delete(key);
}

export function getFocus(key: string): string | null {
  return focus.get(key) ?? null;
}

export function setFocus(key: string, id: string): void {
  focus.set(key, id);
}

export function clearFeedbackCache(): void {
  lists.clear();
  focus.clear();
}

export function subscribe(fn: (e: FeedbackEvent) => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

/** True when `a` comes after `b` in API order (receivedAt descending, then id descending). */
function isAfter(a: Feedback, b: Feedback): boolean {
  return a.receivedAt < b.receivedAt || (a.receivedAt === b.receivedAt && a.id < b.id);
}

/** Pure: what a list looks like after these items changed. */
export function applyToList(
  list: { items: Feedback[]; nextCursor: string | null },
  filters: Filters,
  changed: Feedback[],
): { items: Feedback[]; nextCursor: string | null } {
  let items = list.items;
  for (const c of changed) {
    const idx = items.findIndex((i) => i.id === c.id);
    if (!matchesFilters(c, filters)) {
      if (idx >= 0) items = items.filter((i) => i.id !== c.id);
      continue;
    }
    if (idx >= 0) {
      items = items.map((i, n) => (n === idx ? c : i));
      continue;
    }
    const last = items[items.length - 1];
    // Older than the loaded window: it belongs to a page that is not loaded yet.
    if (list.nextCursor !== null && last !== undefined && isAfter(c, last)) continue;
    const at = items.findIndex((i) => isAfter(i, c));
    items = at < 0 ? [...items, c] : [...items.slice(0, at), c, ...items.slice(at)];
  }
  return { items, nextCursor: list.nextCursor };
}

export function applyFeedback(items: Feedback[], reason: "change" | "undo" | "tags"): void {
  for (const [key, list] of Array.from(lists)) {
    lists.set(key, { ...list, ...applyToList(list, list.filters, items) });
  }
  emit({ type: "apply", items, reason });
}

export function removeFeedback(ids: string[]): void {
  const gone = new Set(ids);
  for (const [key, list] of Array.from(lists)) {
    lists.set(key, { ...list, items: list.items.filter((i) => !gone.has(i.id)) });
  }
  emit({ type: "remove", ids });
}

export function appendPage(key: string, page: FeedbackPage): void {
  const list = lists.get(key);
  if (list === undefined) return;
  const seen = new Set(list.items.map((i) => i.id));
  lists.set(key, {
    ...list,
    items: [...list.items, ...page.items.filter((i) => !seen.has(i.id))],
    nextCursor: page.nextCursor,
  });
  emit({ type: "page", key });
}
