import { useCallback, useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent, type ReactNode, type RefObject } from "react";
import { useNavigate } from "react-router-dom";
import { useAnnounce } from "../components/Announcer";
import { Button } from "../components/Button";
import { Checkbox } from "../components/Checkbox";
import { InlineAlert } from "../components/InlineAlert";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { addTags, bulkStatus, setStatus } from "../lib/endpoints";
import { shortcutFor, type KeyLike } from "../lib/shortcuts";
import type { Feedback, Filters, Status } from "../lib/types";
import "../styles/feedback.css";
import { BulkBar } from "./BulkBar";
import { FeedbackRow } from "./FeedbackRow";
import { ShortcutHelp } from "./ShortcutHelp";
import { addedTagSentence, markedSentence } from "./statusCopy";
import { useFeedbackList } from "./useFeedbackList";

/** Ref callback the empty state attaches to its heading so focus can move there. */
export type EmptyRef = (el: HTMLElement | null) => void;

interface FeedbackListProps {
  filters: Filters;
  showChannel: boolean;
  origin: "inbox" | "user";
  /** The search box: "/" focuses it and Escape inside it returns to the current row. */
  searchRef?: RefObject<HTMLInputElement | null>;
  renderEmpty: (ref: EmptyRef) => ReactNode;
  /** Called after a status change, so the page can refresh its counts. */
  onChanged?: () => void;
  /** Called after a bulk tag add, so the page can refresh its tag list. */
  onTagsChanged?: () => void;
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
  searchRef,
  renderEmpty,
  onChanged,
  onTagsChanged,
}: FeedbackListProps) {
  const list = useFeedbackList(filters);
  const { items } = list;
  const navigate = useNavigate();
  const announce = useAnnounce();
  const toast = useToast();

  const [current, setCurrent] = useState<string | null>(list.restoredFocusId);
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [busy, setBusy] = useState(0);
  const [helpOpen, setHelpOpen] = useState(false);
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
  useEffect(() => {
    const id = list.restoredFocusId;
    if (id === null) return;
    const t = setTimeout(() => setFocusReq({ id }), 0);
    return () => clearTimeout(t);
    // Only on mount.
  }, []);

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

  const selectedIds = items.filter((i) => selected.has(i.id)).map((i) => i.id);

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
    setBusy(ids.length);
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
      const excludes = filters.status !== "all" && status !== filters.status;
      const removed = new Set(excludes ? ids : notFound);
      for (const id of notFound) removed.add(id);
      if (removed.size > 0) list.remove([...removed]);
      for (const it of items) {
        if (removed.has(it.id) || !ids.includes(it.id)) continue;
        list.replace(single ?? { ...it, status });
      }
      const remaining = items.filter((i) => !removed.has(i.id));
      if (remaining.length === 0 && list.nextCursor !== null) list.reload();
      setSelected(new Set());
      announce(markedSentence(updated, status, notFound.length));
      onChanged?.();
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
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setBusy(0);
    }
  };

  const addTag = async (tag: string): Promise<boolean> => {
    if (selectedIds.length === 0 || busy > 0) return false;
    setBusy(selectedIds.length);
    try {
      const r = await addTags(selectedIds, [tag]);
      const gone = new Set(r.notFound);
      for (const it of items) {
        if (selected.has(it.id) && !gone.has(it.id)) list.replace(withTag(it, tag));
      }
      announce(addedTagSentence(tag, r.updated));
      onTagsChanged?.();
      return true;
    } catch (e) {
      toast.show(errorMessage(e), "error");
      return false;
    } finally {
      setBusy(0);
    }
  };

  const toggle = (id: string) =>
    setSelected((cur) => {
      const next = new Set(cur);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const onKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    handleListKey(e, e.target);
  };

  // List shortcuts from inside the list, or from the page itself when nothing else has focus
  // (after load or after focus moved to the h1), so j/k work without tabbing into the list first.
  const handleListKey = (
    e: { key: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; preventDefault: () => void },
    target: EventTarget | null,
  ) => {
    const action = shortcutFor(keyLike(e, target), { dialogOpen: dialogOpen() });
    if (action === null || action === "focusSearch" || action === "toggleHelp") return;
    switch (action) {
      case "next":
        if (e.key === "ArrowDown") e.preventDefault();
        moveBy(1);
        return;
      case "prev":
        if (e.key === "ArrowUp") e.preventDefault();
        moveBy(-1);
        return;
      case "toggleSelect":
        if (currentId !== null) toggle(currentId);
        return;
      case "open":
        if (currentId !== null) {
          markCurrent(currentId);
          navigate(`/feedback/${encodeURIComponent(currentId)}`, { state: { fromList: origin } });
        }
        return;
      case "clearSelection":
        if (selected.size > 0) {
          e.preventDefault();
          setSelected(new Set());
        }
        return;
      default:
        void applyStatus(action);
    }
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

  const allSelected = items.length > 0 && selectedIds.length === items.length;

  return (
    <div className="feedback-list" onKeyDown={onKeyDown}>
      {selectedIds.length > 0 ? (
        <BulkBar
          count={selectedIds.length}
          busy={busy}
          onStatus={(s) => void applyStatus(s)}
          onAddTag={addTag}
          onClear={() => setSelected(new Set())}
        />
      ) : null}
      {list.error ? (
        <InlineAlert onRetry={list.reload}>Could not load feedback. {list.error}</InlineAlert>
      ) : null}
      {items.length > 0 ? (
        <>
          <div className="list-head">
            <Checkbox
              label="Select all on this page"
              checked={allSelected}
              indeterminate={selectedIds.length > 0 && !allSelected}
              onChange={() => setSelected(allSelected ? new Set() : new Set(items.map((i) => i.id)))}
            />
          </div>
          <ul className="rows" aria-label="Feedback" ref={listRef}>
            {items.map((item) => (
              <FeedbackRow
                key={item.id}
                item={item}
                showChannel={showChannel}
                selected={selected.has(item.id)}
                current={item.id === currentId}
                origin={origin}
                onToggle={() => toggle(item.id)}
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
        <p className="muted">Loading feedback…</p>
      ) : list.error ? null : (
        renderEmpty(emptyRef)
      )}
      <ShortcutHelp open={helpOpen} onToggle={() => setHelpOpen((o) => !o)} />
    </div>
  );
}
