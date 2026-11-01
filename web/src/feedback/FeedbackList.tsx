import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode, type RefObject } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAnnounce } from "../components/Announcer";
import { Button } from "../components/Button";
import { InlineAlert } from "../components/InlineAlert";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { addTags, bulkStatus, removeTags, setStatus } from "../lib/endpoints";
import { matchesFilters } from "../lib/filters";
import { shortcutFor, type KeyLike, type ShortcutAction } from "../lib/shortcuts";
import type { Feedback, Filters, Status, TagCount } from "../lib/types";
import "../styles/feedback.css";
import { FeedbackRow } from "./FeedbackRow";
import { ListHeader, type FilterChip, type SelectNotice } from "./ListHeader";
import { ShortcutHelp } from "./ShortcutHelp";
import { applyFeedback, getList, listKey, removeFeedback, subscribe } from "./listStore";
import type { FromList } from "./origin";
import { SELECT_CAP, selectMatching, type Matching } from "./selectMatching";
import { addedTagSentence, markedSentence, markedToast, withUndoHint } from "./statusCopy";
import { useUndo } from "./undo";
import { useDelayedFlag, useFeedbackList } from "./useFeedbackList";
import { useKeyQueue } from "./useKeyQueue";
import { useNow } from "./useNow";

/** Ref callback the empty state attaches to its heading so focus can move there. */
export type EmptyRef = (el: HTMLElement | null) => void;

interface FeedbackListProps {
  filters: Filters;
  showChannel: boolean;
  origin: "inbox" | "user";
  /** Shown on the detail page's Back button: "Inbox" or the user ID. */
  originLabel: string;
  /** The search box: "/" focuses it and Escape inside it returns to the current row. */
  searchRef?: RefObject<HTMLInputElement | null>;
  renderEmpty: (ref: EmptyRef) => ReactNode;
  /** The active tab's count, when known. Unknown shows how many are loaded. */
  count?: number | null;
  /** One removable chip per active non-status filter (the inbox passes them; the per-user page none). */
  chips?: FilterChip[];
  onClearAll?: () => void;
  /** Existing tags with their counts, offered by the bulk bar's tag editor. */
  tags?: TagCount[];
}

function keyLike(e: { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean }, target: EventTarget | null): KeyLike {
  const t = target instanceof HTMLElement ? target : null;
  return {
    key: e.key,
    ctrlKey: e.ctrlKey,
    metaKey: e.metaKey,
    altKey: e.altKey,
    target: t
      ? {
          tagName: t.tagName,
          isContentEditable: t.isContentEditable,
          type: t instanceof HTMLInputElement ? t.type : undefined,
        }
      : null,
  };
}

/** How long a request runs before the list says so. */
const LOADING_DELAY_MS = 300;

const dialogOpen = () => document.querySelector('[aria-modal="true"]') !== null;

function withTag(item: Feedback, tag: string): Feedback {
  if (item.tags.some((t) => t.toLowerCase() === tag.toLowerCase())) return item;
  const tags = [...item.tags, tag].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
  return { ...item, tags };
}

export function FeedbackList({
  filters,
  showChannel,
  origin,
  originLabel,
  searchRef,
  renderEmpty,
  count = null,
  chips = [],
  onClearAll,
  tags = [],
}: FeedbackListProps) {
  const list = useFeedbackList(filters);
  const { items } = list;
  const navigate = useNavigate();
  const announce = useAnnounce();
  const toast = useToast();
  const undo = useUndo();
  const location = useLocation();
  const key = listKey(filters);
  const from: FromList = {
    kind: origin,
    key,
    filters,
    path: location.pathname + location.search,
    label: originLabel,
  };

  const [current, setCurrent] = useState<string | null>(list.restoredFocusId);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  // Set after "Select all matching": every selected item, loaded or not.
  const [matching, setMatching] = useState<Matching | null>(null);
  const [selectingAll, setSelectingAll] = useState(false);
  const anchor = useRef<{ id: string; on: boolean } | null>(null);
  const now = useNow();
  const [busy, setBusy] = useState(0);
  const [restoreIds, setRestoreIds] = useState<ReadonlySet<string> | null>(null);
  const [helpOpen, setHelpOpen] = useState(false);
  // A "t" press asks the bulk bar to open its tag popover; the popover clears the request.
  const [tagOpen, setTagOpen] = useState(false);
  const [seenKey, setSeenKey] = useState(key);
  const stale = list.stale;
  // The list stays mounted when the query changes, so what used to reset by remounting resets here:
  // selection, the current row and the select-all notice (the anchor and pending `j` in the effect below).
  if (seenKey !== key) {
    setSeenKey(key);
    setCurrent(list.restoredFocusId);
    setSelected(new Set());
    setMatching(null);
    setSelectingAll(false);
    setTagOpen(false);
  }
  const [focusReq, setFocusReq] = useState<{ id: string | null } | null>(null);
  const listRef = useRef<HTMLUListElement | null>(null);
  const emptyEl = useRef<HTMLElement | null>(null);
  const emptyRef = useCallback<EmptyRef>((el) => {
    emptyEl.current = el;
  }, []);
  const pendingNext = useRef<string | null>(null);

  const currentId = items.some((i) => i.id === current) ? current : (items[0]?.id ?? null);
  const { setFocused } = list;

  const markCurrent = useCallback(
    (id: string) => {
      setCurrent(id);
      setFocused(id);
    },
    [setFocused],
  );

  const focusRow = useCallback(
    (id: string | null) => {
      if (id !== null) markCurrent(id);
      setFocusReq({ id });
    },
    [markCurrent],
  );

  // Move DOM focus after the render that changed the current row or emptied the list.
  useEffect(() => {
    if (focusReq === null) return;
    if (focusReq.id === null) emptyEl.current?.focus();
    else {
      for (const li of Array.from(listRef.current?.children ?? [])) {
        if (li instanceof HTMLElement && li.dataset["id"] === focusReq.id) {
          li.querySelector<HTMLAnchorElement>("a.row-link")?.focus();
          break;
        }
      }
    }
    setFocusReq(null);
  }, [focusReq]);

  // Back from a detail page: put focus on the row that was opened, after the page's own h1 focus.
  // Runs on mount and whenever the query changes (the list no longer remounts per query), but never
  // takes focus from a field the user is typing in.
  useEffect(() => {
    anchor.current = null;
    pendingNext.current = null;
    const id = list.restoredFocusId;
    if (id === null) return;
    const t = setTimeout(() => {
      const a = document.activeElement;
      if (a instanceof HTMLElement && (a.tagName === "INPUT" || a.tagName === "SELECT" || a.tagName === "TEXTAREA")) {
        if (!(a instanceof HTMLInputElement && (a.type === "checkbox" || a.type === "radio"))) return;
      }
      setFocusReq({ id });
    }, 0);
    return () => clearTimeout(t);
    // Per query, not per render.
  }, [key]);

  // After Undo, focus the first restored row that is in the list.
  useEffect(
    () =>
      subscribe((e) => {
        if (e.type === "apply" && e.reason === "undo") setRestoreIds(new Set(e.items.map((i) => i.id)));
      }),
    [],
  );
  useEffect(() => {
    if (restoreIds === null) return;
    const first = items.find((i) => restoreIds.has(i.id));
    setRestoreIds(null);
    if (first) focusRow(first.id);
  }, [restoreIds, items, focusRow]);

  // `j` on the last row loads more, then moves on to the first new row.
  const { loading, loadMore } = list;
  useEffect(() => {
    const prev = pendingNext.current;
    if (prev === null) return;
    const idx = items.findIndex((i) => i.id === prev);
    if (idx >= 0 && idx + 1 < items.length) {
      pendingNext.current = null;
      focusRow(items[idx + 1]?.id ?? null);
    } else if (!loading) {
      pendingNext.current = null;
    }
  }, [items, loading, focusRow]);

  const selectedItems = matching !== null ? matching.items : items.filter((i) => selected.has(i.id));
  const selectedIds = selectedItems.map((i) => i.id);

  const changeSelection = (next: ReadonlySet<string>) => {
    setMatching(null);
    setSelected(next);
  };

  const moveBy = (delta: number) => {
    const idx = items.findIndex((i) => i.id === currentId);
    if (idx < 0) {
      focusRow(items[0]?.id ?? null);
      return;
    }
    const target = idx + delta;
    if (target >= items.length) {
      if (list.nextCursor !== null) {
        pendingNext.current = items[idx]?.id ?? null;
        loadMore();
      }
      return;
    }
    if (target >= 0) focusRow(items[target]?.id ?? null);
  };

  const applyStatus = async (status: Status) => {
    if (busy > 0) return;
    const bulk = selectedIds.length > 0;
    const ids = bulk ? selectedIds : currentId !== null ? [currentId] : [];
    if (ids.length === 0) return;
    if (!queue.begin()) return;
    setBusy(ids.length);
    const idSet = new Set(ids);
    const before = bulk ? selectedItems : items.filter((i) => idSet.has(i.id));
    const run = async (): Promise<boolean> => {
      try {
        let updated = ids.length;
        let notFound: string[] = [];
        let single: Feedback | null = null;
        if (bulk) {
          const r = await bulkStatus(ids, status);
          updated = r.updated;
          notFound = r.notFound;
        } else {
          single = await setStatus(ids[0] ?? "", status);
        }
        const gone = new Set(notFound);
        const changed = before.filter((it) => !gone.has(it.id)).map((it) => single ?? { ...it, status });
        const removed = new Set(notFound);
        for (const c of changed) if (!matchesFilters(c, filters)) removed.add(c.id);
        applyFeedback(changed, "change");
        if (notFound.length > 0) removeFeedback(notFound);
        const left = getList(key);
        if (left && left.items.length === 0 && left.nextCursor !== null) list.reload();
        changeSelection(new Set());
        announce(withUndoHint(markedSentence(updated, status, notFound.length)));
        if (updated > 0) {
          undo.offer({ message: markedToast(updated, status), before: before.filter((it) => !gone.has(it.id)) });
        }
        if (currentId !== null && removed.has(currentId)) {
          const idx = items.findIndex((i) => i.id === currentId);
          const next =
            items.slice(idx + 1).find((i) => !removed.has(i.id)) ??
            items
              .slice(0, idx)
              .reverse()
              .find((i) => !removed.has(i.id));
          focusRow(next?.id ?? null);
        } else if (bulk) {
          focusRow(currentId);
        }
        return true;
      } catch (e) {
        toast.show(errorMessage(e), "error");
        return false;
      } finally {
        setBusy(0);
        queue.end();
      }
    };
    await undo.track(run());
  };

  const addTag = async (tag: string): Promise<boolean> => {
    if (selectedIds.length === 0 || busy > 0) return false;
    setBusy(selectedIds.length);
    try {
      const r = await addTags(selectedIds, [tag]);
      const gone = new Set(r.notFound);
      applyFeedback(
        selectedItems.filter((it) => !gone.has(it.id)).map((it) => withTag(it, tag)),
        "tags",
      );
      announce(addedTagSentence(tag, r.updated));
      return true;
    } catch (e) {
      toast.show(errorMessage(e), "error");
      return false;
    } finally {
      setBusy(0);
    }
  };

  const removeTag = async (tag: string) => {
    const carrying = selectedItems.filter((it) => it.tags.includes(tag));
    if (carrying.length === 0 || busy > 0) return;
    setBusy(carrying.length);
    try {
      const r = await removeTags(
        carrying.map((it) => it.id),
        [tag],
      );
      const gone = new Set(r.notFound);
      applyFeedback(
        carrying.filter((it) => !gone.has(it.id)).map((it) => ({ ...it, tags: it.tags.filter((t) => t !== tag) })),
        "tags",
      );
      const removed = carrying.filter((it) => !gone.has(it.id));
      announce(withUndoHint(`Removed tag ${tag} from ${r.updated}.`));
      if (removed.length > 0) {
        undo.offer({ kind: "tag-remove", message: `Removed tag ${tag} from ${removed.length}`, tag, before: removed });
      }
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setBusy(0);
    }
  };

  // Click or x toggles a row and sets the anchor. With `range`, every loaded row between the anchor
  // and this one takes the anchor's new state.
  const toggle = (id: string, range = false) => {
    if (stale) return;
    const from = range && anchor.current !== null ? anchor.current : null;
    const a = from === null ? -1 : items.findIndex((i) => i.id === from.id);
    const b = items.findIndex((i) => i.id === id);
    if (from !== null && a >= 0 && b >= 0) {
      const next = new Set(selected);
      for (const it of items.slice(Math.min(a, b), Math.max(a, b) + 1)) {
        if (from.on) next.add(it.id);
        else next.delete(it.id);
      }
      anchor.current = { id, on: from.on };
      changeSelection(next);
      return;
    }
    const on = !selected.has(id);
    const next = new Set(selected);
    if (on) next.add(id);
    else next.delete(id);
    anchor.current = { id, on };
    changeSelection(next);
  };

  const selectAllMatching = async () => {
    if (selectingAll) return;
    setSelectingAll(true);
    try {
      const r = await selectMatching(filters, { items, nextCursor: list.nextCursor });
      setMatching(r);
      setSelected(new Set(r.items.map((i) => i.id)));
      announce(r.capped ? `Selected the first ${r.items.length} matching.` : `Selected all ${r.items.length} matching.`);
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setSelectingAll(false);
    }
  };

  const onKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    handleListKey(e, e.target);
  };

  // Runs a list action against the current state. Status actions start a request.
  const runAction = (action: ShortcutAction) => {
    if (stale) return;
    switch (action) {
      case "next":
        moveBy(1);
        return;
      case "prev":
        moveBy(-1);
        return;
      case "toggleSelect":
        if (currentId !== null) toggle(currentId);
        return;
      case "rangeSelect":
        if (currentId !== null) toggle(currentId, true);
        return;
      case "open":
        if (currentId !== null) {
          markCurrent(currentId);
          navigate(`/feedback/${encodeURIComponent(currentId)}`, { state: { from } });
        }
        return;
      case "clearSelection":
        if (selected.size > 0) changeSelection(new Set());
        return;
      case "reviewed":
      case "actioned":
      case "ignored":
      case "new":
        void applyStatus(action);
        return;
      case "tag":
        if (busy > 0) return;
        if (selectedIds.length === 0) {
          if (currentId === null) return;
          changeSelection(new Set([currentId]));
        }
        setTagOpen(true);
        return;
      default:
        // focusSearch and toggleHelp are the document handler's; undo and back belong elsewhere.
        return;
    }
  };

  // Keys pressed while a status request runs wait and run in order afterwards.
  const queue = useKeyQueue<ShortcutAction>((a) => runActionRef.current(a));
  const runActionRef = useRef(runAction);
  runActionRef.current = runAction;

  // List shortcuts from inside the list, or from the page itself when nothing else has focus
  // (after load or after focus moved to the h1), so j/k work without tabbing into the list first.
  const handleListKey = (
    e: { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; preventDefault: () => void },
    target: EventTarget | null,
  ) => {
    const action = shortcutFor(keyLike(e, target), { dialogOpen: dialogOpen() });
    if (action === null || action === "focusSearch" || action === "toggleHelp" || action === "undo" || action === "back") {
      return;
    }
    if (action === "next" && e.key === "ArrowDown") e.preventDefault();
    if (action === "prev" && e.key === "ArrowUp") e.preventDefault();
    if (action === "clearSelection" && selected.size > 0) e.preventDefault();
    if (action === "tag") e.preventDefault();
    if (queue.offer(action)) return;
    runAction(action);
  };

  const handleKeyRef = useRef(handleListKey);
  handleKeyRef.current = handleListKey;

  // "/" and "?" work anywhere on the page; Escape in the search box returns to the current row.
  useEffect(() => {
    const onDoc = (e: KeyboardEvent) => {
      const box = searchRef?.current;
      if (e.key === "Escape" && box && e.target === box) {
        e.preventDefault();
        box.blur();
        focusRow(currentId);
        return;
      }
      const t = e.target;
      if (t === document.body || (t instanceof HTMLElement && (t.tagName === "MAIN" || t.tagName === "H1"))) {
        const listAction = shortcutFor(keyLike(e, t), { dialogOpen: dialogOpen() });
        if (listAction === "next" || listAction === "prev") {
          // First press from outside the list lands on the current row.
          e.preventDefault();
          focusRow(currentId);
          return;
        }
        handleKeyRef.current(e, t);
      }
      const action = shortcutFor(keyLike(e, e.target), { dialogOpen: dialogOpen() });
      if (action === "focusSearch" && box) {
        e.preventDefault();
        box.focus();
      } else if (action === "toggleHelp") {
        e.preventDefault();
        setHelpOpen((o) => !o);
      }
    };
    document.addEventListener("keydown", onDoc);
    return () => document.removeEventListener("keydown", onDoc);
  }, [searchRef, currentId, focusRow]);

  const allSelected = items.length > 0 && items.every((i) => selected.has(i.id));
  const total = count ?? (list.nextCursor === null ? items.length : null);

  let notice: SelectNotice | null = null;
  if (matching !== null) {
    notice = {
      kind: "done",
      text: matching.capped
        ? `The first ${SELECT_CAP} matching are selected. Bulk actions handle up to ${SELECT_CAP} at a time.`
        : `All ${matching.items.length} matching are selected.`,
      onClear: () => changeSelection(new Set()),
    };
  } else if (allSelected && list.nextCursor !== null) {
    notice = {
      kind: "offer",
      loaded: items.length,
      action:
        count === null
          ? `Select all matching, up to ${SELECT_CAP}`
          : count > SELECT_CAP
            ? `Select the first ${SELECT_CAP} matching`
            : `Select all ${count} matching`,
      busy: selectingAll,
      onSelect: () => void selectAllMatching(),
    };
  }

  const slowLoad = useDelayedFlag(list.loading && items.length === 0, LOADING_DELAY_MS);
  const slowStale = useDelayedFlag(stale, LOADING_DELAY_MS);
  const pending = slowStale
    ? list.staleFilters !== null && list.staleFilters.q.trim() !== filters.q.trim()
      ? "Searching…"
      : "Loading…"
    : null;

  return (
    <div className="feedback-list" onKeyDown={onKeyDown}>
      {list.error ? (
        <InlineAlert onRetry={list.reload}>Could not load feedback. {list.error}</InlineAlert>
      ) : null}
      {items.length > 0 ? (
        <>
          <ListHeader
            total={total}
            loaded={items.length}
            pending={pending}
            chips={chips}
            onClearAll={onClearAll ?? (() => undefined)}
            status={filters.status}
            allSelected={allSelected}
            selectedItems={selectedItems}
            selectedCount={selectedIds.length}
            busy={busy}
            tags={tags}
            notice={notice}
            tagRequested={tagOpen}
            onTagOpened={() => setTagOpen(false)}
            onToggleAll={() => {
              if (stale) return;
              anchor.current = null;
              changeSelection(allSelected ? new Set() : new Set(items.map((i) => i.id)));
            }}
            onStatus={(st) => void applyStatus(st)}
            onAddTag={addTag}
            onRemoveTag={(t) => void removeTag(t)}
            onClear={() => changeSelection(new Set())}
          />
          <ul className="rows" aria-label="Feedback" ref={listRef} {...(stale ? { "aria-busy": true } : {})}>
            {items.map((item) => (
              <FeedbackRow
                key={item.id}
                item={item}
                showStatus={filters.status === "all"}
                showChannel={showChannel && filters.channel === ""}
                showUser={origin !== "user"}
                now={now}
                selected={selected.has(item.id)}
                current={item.id === currentId}
                from={from}
                onToggle={(range) => toggle(item.id, range)}
                onCurrent={() => markCurrent(item.id)}
              />
            ))}
          </ul>
          {list.nextCursor !== null ? (
            <div className="load-more">
              <Button variant="text" loading={list.loading} loadingLabel="Loading…" onClick={loadMore}>
                Load more
              </Button>
              <p className="muted">Showing {items.length}</p>
            </div>
          ) : null}
        </>
      ) : list.loading ? (
        slowLoad ? (
          <p className="muted">Loading feedback…</p>
        ) : null
      ) : list.error ? null : (
        renderEmpty(emptyRef)
      )}
      <ShortcutHelp open={helpOpen} onToggle={() => setHelpOpen((o) => !o)} />
    </div>
  );
}
