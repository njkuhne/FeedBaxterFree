const STORAGE_KEY = "feedbaxter.pending.v1";

export interface Item {
  idempotencyKey: string;
  body: Record<string, unknown>;
  key?: string;
  notBefore?: number;
}

export interface Backlog {
  list(): Item[];
  has(idempotencyKey: string): boolean;
  put(item: Item): void;
  remove(idempotencyKey: string): void;
  setNotBefore(idempotencyKey: string, notBefore: number): void;
}

function valid(v: unknown): v is Item {
  const o = v as Partial<Item> | null;
  return !!o && typeof o === "object" && typeof o.idempotencyKey === "string" && !!o.body && typeof o.body === "object";
}

function size(items: Item[]): number {
  return new TextEncoder().encode(JSON.stringify(items)).length;
}

/** A backlog over `storage`. Every storage access is guarded, and a failure means "no backlog" for that call. */
export function createBacklog(storage: Storage | null, limit: number, bytes: number): Backlog {
  const s = limit > 0 && bytes > 0 ? storage : null;

  function load(): Item[] {
    if (!s) return [];
    try {
      const raw = s.getItem(STORAGE_KEY);
      const v: unknown = raw ? JSON.parse(raw) : [];
      return Array.isArray(v) ? v.filter(valid) : [];
    } catch {
      return [];
    }
  }

  function save(items: Item[]): void {
    if (!s) return;
    try {
      if (items.length === 0) s.removeItem(STORAGE_KEY);
      else s.setItem(STORAGE_KEY, JSON.stringify(items));
    } catch {
      // Quota or blocked storage: the item is simply not queued.
    }
  }

  return {
    list: load,
    has: (k) => load().some((i) => i.idempotencyKey === k),
    put(item) {
      if (!s || size([item]) > bytes) return;
      const items = load();
      const at = items.findIndex((i) => i.idempotencyKey === item.idempotencyKey);
      if (at >= 0) items[at] = item;
      else items.push(item);
      while (items.length > limit || size(items) > bytes) items.shift();
      save(items);
    },
    remove(k) {
      const items = load();
      const rest = items.filter((i) => i.idempotencyKey !== k);
      if (rest.length !== items.length) save(rest);
    },
    setNotBefore(k, notBefore) {
      const items = load();
      const it = items.find((i) => i.idempotencyKey === k);
      if (it) {
        it.notBefore = notBefore;
        save(items);
      }
    },
  };
}
