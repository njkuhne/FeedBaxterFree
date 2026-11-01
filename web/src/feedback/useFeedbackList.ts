import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { errorMessage } from "../lib/api";
import { cursorReducer, type CursorAction, type CursorState } from "../lib/cursorList";
import { listFeedback } from "../lib/endpoints";
import type { Feedback, Filters } from "../lib/types";
import {
  applyToList,
  clearFeedbackCache,
  dropList,
  getFocus,
  getList,
  listKey,
  setFocus,
  setList,
  subscribe,
} from "./listStore";

export { clearFeedbackCache };

interface Cached {
  items: Feedback[];
  nextCursor: string | null;
}

type ListState = CursorState<Feedback> & { key: string };

type ListAction =
  | CursorAction<Feedback>
  | { type: "begin"; key: string; requestId: number; seed: Cached | null }
  | { type: "more"; requestId: number }
  | { type: "apply"; items: Feedback[]; filters: Filters }
  | { type: "cache"; items: Feedback[]; nextCursor: string | null }
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
    case "apply":
      return { ...state, ...applyToList(state, action.filters, action.items) };
    case "cache":
      return { ...state, items: action.items, nextCursor: action.nextCursor, loading: false };
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
  const cached = getList(key);
  return {
    key,
    items: cached?.items ?? [],
    nextCursor: cached?.nextCursor ?? null,
    loading: true,
    error: null,
    requestId: 0,
  };
}

/** True once `active` has stayed true for `ms`; false at once when it goes false. */
export function useDelayedFlag(active: boolean, ms: number): boolean {
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (!active) {
      setOn(false);
      return;
    }
    const t = setTimeout(() => setOn(true), ms);
    return () => clearTimeout(t);
  }, [active, ms]);
  return active && on;
}

export interface FeedbackListApi {
  /** The rows to show: this query's, or while it loads with nothing cached, the previous query's (see `stale`). */
  items: Feedback[];
  /** True while `items` belong to the previous query and the new one is still loading. */
  stale: boolean;
  /** The previous query's filters when `stale`, so the caller can tell a search from another change. */
  staleFilters: Filters | null;
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
  const key = listKey(filters);
  const [state, dispatch] = useReducer(listReducer, key, init);
  // The focused row remembered for this query. Recomputed when the query changes, since the list
  // component no longer remounts per query.
  const restored = useRef<{ key: string; id: string | null }>({ key, id: getFocus(key) });
  if (restored.current.key !== key) restored.current = { key, id: getFocus(key) };
  const shown = useRef<{ key: string; filters: Filters; items: Feedback[] } | null>(null);
  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  const stateRef = useRef(state);
  stateRef.current = state;
  const requestId = useRef(0);
  const [tick, setTick] = useState(0);

  useEffect(() => {
    const rid = ++requestId.current;
    const cached = getList(key) ?? null;
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
      setList(key, { filters: filtersRef.current, items: state.items, nextCursor: state.nextCursor });
    }
  }, [state.items, state.nextCursor, state.loading, state.key, key]);

  // Changes made anywhere (the list, the detail page, Undo) arrive through the store.
  useEffect(
    () =>
      subscribe((e) => {
        if (e.type === "apply") dispatch({ type: "apply", items: e.items, filters: filtersRef.current });
        else if (e.type === "remove") dispatch({ type: "remove", ids: e.ids });
        else if (e.key === key) {
          const c = getList(key);
          if (c) dispatch({ type: "cache", items: c.items, nextCursor: c.nextCursor });
        }
      }),
    [key],
  );

  const loadMore = useCallback(() => {
    const s = stateRef.current;
    if (s.key !== listKey(filtersRef.current) || s.loading || s.nextCursor === null) return;
    const rid = ++requestId.current;
    dispatch({ type: "more", requestId: rid });
    listFeedback(filtersRef.current, s.nextCursor)
      .then((page) => dispatch({ type: "loaded", requestId: rid, items: page.items, nextCursor: page.nextCursor }))
      .catch((e: unknown) => dispatch({ type: "failed", requestId: rid, error: errorMessage(e) }));
  }, []);

  const reload = useCallback(() => {
    dropList(key);
    setTick((t) => t + 1);
  }, [key]);

  const remove = useCallback((ids: string[]) => dispatch({ type: "remove", ids }), []);
  const replace = useCallback((item: Feedback) => dispatch({ type: "replace", item }), []);
  const setFocused = useCallback(
    (id: string) => {
      setFocus(key, id);
    },
    [key],
  );

  const ready = state.key === key;
  const own = ready && (state.items.length > 0 || !state.loading);
  if (own) shown.current = { key, filters: filtersRef.current, items: state.items };
  const keep = !own && shown.current !== null && shown.current.items.length > 0 ? shown.current : null;
  return {
    items: own ? state.items : keep !== null ? keep.items : [],
    stale: keep !== null,
    staleFilters: keep !== null ? keep.filters : null,
    nextCursor: ready ? state.nextCursor : null,
    loading: ready ? state.loading : true,
    error: ready ? state.error : null,
    restoredFocusId: restored.current.id,
    loadMore,
    reload,
    remove,
    replace,
    setFocused,
  };
}
