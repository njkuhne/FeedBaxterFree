import { listFeedback } from "../lib/endpoints";
import type { Feedback, Filters } from "../lib/types";

export const SELECT_CAP = 500;
const PAGE = 200;

export interface Loaded {
  items: Feedback[];
  nextCursor: string | null;
}

export interface Matching {
  items: Feedback[];
  /** True when more items match than the cap allowed. */
  capped: boolean;
}

/**
 * Pages the items that match `filters` client-side, continuing after what the list already loaded,
 * until the cursor runs out or `cap` items are collected. It does not touch the list itself.
 */
export async function selectMatching(filters: Filters, loaded: Loaded, cap = SELECT_CAP): Promise<Matching> {
  const seen = new Set<string>();
  const items: Feedback[] = [];
  let overflow = false;
  const add = (list: Feedback[]) => {
    for (const it of list) {
      if (!seen.has(it.id)) {
        if (items.length >= cap) {
          overflow = true;
          return;
        }
        seen.add(it.id);
        items.push(it);
      }
    }
  };
  add(loaded.items);
  let cursor = loaded.nextCursor;
  while (cursor !== null && items.length < cap) {
    const page = await listFeedback(filters, cursor, PAGE);
    add(page.items);
    cursor = page.nextCursor;
  }
  return { items, capped: overflow || cursor !== null };
}
