import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { errorMessage } from "../lib/api";
import { cursorReducer, type CursorAction, type CursorState } from "../lib/cursorList";
import { listFeedback } from "../lib/endpoints";
import { toApiParams } from "../lib/filters";
import type { Feedback, Filters } from "../lib/types";

interface Cached {
  items: Feedback[];
  nextCursor: string | null;
}

// In-memory only: lists and the focused row per query, so Back from a detail page restores them.
const listCache = new Map<string, Cached>();
const focusCache = new Map<string, string>();

export function clearFeedbackCache(): void {
  listCache.clear();
  focusCache.clear();
}

type ListState = CursorState<Feedback> & { key: string };

type ListAction =
  | CursorAction<Feedback>
  | { type: "begin"; key: string; requestId: number; seed: Cached | null }
  | { type: "more"; requestId: number }
  | { type: "refreshed"; requestId: number; items: Feedback[]; nextCursor: string | null };

function isAfter(a: Feedback, b: Feedback): boolean {
  return a.receivedAt < b.receivedAt || (a.receivedAt === b.receivedAt && a.id < b.id);
}

function listReducer(state: ListState, action: ListAction): ListState {
  switch (action.type) {
    case "begin":
      return {
        key: action.key,
        items: action.seed?.items ?? [],
        nextCursor: action.seed?.nextCursor ?? null,
        loading: true,
        error: null,
        requestId: action.requestId,
      };
    case "more":
      return { ...state, loading: true, error: null, requestId: action.requestId };
    case "refreshed": {
      if (action.requestId !== state.requestId) return state;
      // Fresh first page, then whatever older rows were already loaded, so the focused row survives.
      const fresh = action.items;
      const seen = new Set(fresh.map((i) => i.id));
      const last = fresh[fresh.length - 1];
      const tail =
        action.nextCursor === null || last === undefined
          ? []
          : state.items.filter((i) => !seen.has(i.id) && isAfter(i, last));
      return {
        ...state,
        items: [...fresh, ...tail],
        nextCursor: tail.length > 0 ? state.nextCursor : action.nextCursor,
        loading: false,
        error: null,
      };
    }
    default:
      return { ...cursorReducer<Feedback>(state, action), key: state.key };
  }
}

function init(key: string): ListState {
  const cached = listCache.get(key);
  return {
    key,
    items: cached?.items ?? [],
    nextCursor: cached?.nextCursor ?? null,
    loading: true,
    error: null,
    requestId: 0,
  };
}

export interface FeedbackListApi {
  items: Feedback[];
  nextCursor: string | null;
  loading: boolean;
  error: string | null;
  /** The row that had focus when the list was last left, if this query was cached. */
  restoredFocusId: string | null;
  loadMore: () => void;
  reload: () => void;
  remove: (ids: string[]) => void;
  replace: (item: Feedback) => void;
  setFocused: (id: string) => void;
}

/** Fetches a feedback list by cursor, with an in-memory cache keyed by the query. */
export function useFeedbackList(filters: Filters): FeedbackListApi {
  const key = toApiParams(filters).toString();
  const [state, dispatch] = useReducer(listReducer, key, init);
  const restoredFocusId = useRef<string | null>(focusCache.get(key) ?? null);
  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  const stateRef = useRef(state);
  stateRef.current = state;
  const requestId = useRef(0);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const rid = ++requestId.current;
    const cached = listCache.get(key) ?? null;
    dispatch({ type: "begin", key, requestId: rid, seed: cached });
    listFeedback(filtersRef.current)
      .then((page) => {
        dispatch(
          cached
            ? { type: "refreshed", requestId: rid, items: page.items, nextCursor: page.nextCursor }
            : { type: "loaded", requestId: rid, items: page.items, nextCursor: page.nextCursor },
        );
      })
      .catch((e: unknown) => dispatch({ type: "failed", requestId: rid, error: errorMessage(e) }));
  }, [key, tick]);

  useEffect(() => {
    if (state.key === key && (state.items.length > 0 || !state.loading)) {
      listCache.set(key, { items: state.items, nextCursor: state.nextCursor });
    }
  }, [state.items, state.nextCursor, state.loading, state.key, key]);

  const loadMore = useCallback(() => {
    const s = stateRef.current;
    if (s.loading || s.nextCursor === null) return;
    const rid = ++requestId.current;
    dispatch({ type: "more", requestId: rid });
    listFeedback(filtersRef.current, s.nextCursor)
      .then((page) => dispatch({ type: "loaded", requestId: rid, items: page.items, nextCursor: page.nextCursor }))
      .catch((e: unknown) => dispatch({ type: "failed", requestId: rid, error: errorMessage(e) }));
  }, []);

  const reload = useCallback(() => {
    listCache.delete(key);
    setTick((t) => t + 1);
  }, [key]);

  const remove = useCallback((ids: string[]) => dispatch({ type: "remove", ids }), []);
  const replace = useCallback((item: Feedback) => dispatch({ type: "replace", item }), []);
  const setFocused = useCallback(
    (id: string) => {
      focusCache.set(key, id);
    },
    [key],
  );

  const ready = state.key === key;
  return {
    items: ready ? state.items : [],
    nextCursor: ready ? state.nextCursor : null,
    loading: ready ? state.loading : true,
    error: ready ? state.error : null,
    restoredFocusId: restoredFocusId.current,
    loadMore,
    reload,
    remove,
    replace,
    setFocused,
  };
}
