export interface CursorState<T extends { id: string }> {
  items: T[];
  nextCursor: string | null;
  loading: boolean;
  error: string | null;
  /** Incremented by every request; answers for older ids are ignored. */
  requestId: number;
}

export type CursorAction<T extends { id: string }> =
  | { type: "reset" }
  | { type: "loadMore" }
  | { type: "loaded"; requestId: number; items: T[]; nextCursor: string | null }
  | { type: "failed"; requestId: number; error: string }
  | { type: "remove"; ids: string[] }
  | { type: "replace"; item: T };

export function initialCursorState<T extends { id: string }>(): CursorState<T> {
  return { items: [], nextCursor: null, loading: false, error: null, requestId: 0 };
}

export function cursorReducer<T extends { id: string }>(
  state: CursorState<T>,
  action: CursorAction<T>,
): CursorState<T> {
  switch (action.type) {
    case "reset":
      return { items: [], nextCursor: null, loading: true, error: null, requestId: state.requestId + 1 };
    case "loadMore":
      return { ...state, loading: true, error: null, requestId: state.requestId + 1 };
    case "loaded": {
      if (action.requestId !== state.requestId) return state;
      const seen = new Set(state.items.map((i) => i.id));
      const fresh = action.items.filter((i) => !seen.has(i.id));
      return {
        ...state,
        items: [...state.items, ...fresh],
        nextCursor: action.nextCursor,
        loading: false,
        error: null,
      };
    }
    case "failed":
      if (action.requestId !== state.requestId) return state;
      return { ...state, loading: false, error: action.error };
    case "remove": {
      const gone = new Set(action.ids);
      return { ...state, items: state.items.filter((i) => !gone.has(i.id)) };
    }
    case "replace":
      return { ...state, items: state.items.map((i) => (i.id === action.item.id ? action.item : i)) };
  }
}
