import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAnnounce } from "../components/Announcer";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { addTags, bulkStatus } from "../lib/endpoints";
import { shortcutFor } from "../lib/shortcuts";
import type { Feedback, Status } from "../lib/types";
import { applyFeedback } from "./listStore";

interface UndoBase {
  /** Visible toast text, e.g. "Marked 3 reviewed". */
  message: string;
  /** Snapshots of the items before the change. */
  before: Feedback[];
}

export interface StatusUndo extends UndoBase {
  kind?: "status";
  /** Detail only: after an auto-advance, Undo returns to the restored item if the user is still on `landedOn`. */
  revisit?: { landedOn: string; path: string; state: unknown };
}

/** A tag was removed from the items in `before` (which still carry it). */
export interface TagRemoveUndo extends UndoBase {
  kind: "tag-remove";
  tag: string;
}

export type UndoRecord = StatusUndo | TagRemoveUndo;

interface UndoApi {
  offer: (r: UndoRecord) => void;
  undo: () => Promise<void>;
  available: boolean;
  /** Registers a running status change so that Undo pressed meanwhile waits for it. Resolves to its success. */
  track: (p: Promise<boolean>) => Promise<boolean>;
}

/** Ids grouped by the status they had before the change, in order of first appearance. */
export function groupByStatus(before: Feedback[]): Map<Status, string[]> {
  const groups = new Map<Status, string[]>();
  for (const item of before) {
    const ids = groups.get(item.status);
    if (ids) ids.push(item.id);
    else groups.set(item.status, [item.id]);
  }
  return groups;
}

const UNDO_KEY = "undo";
const UNDO_DURATION = 6000;

const UndoContext = createContext<UndoApi>({
  offer: () => undefined,
  undo: () => Promise.resolve(),
  available: false,
  track: (p) => p,
});

export function useUndo(): UndoApi {
  return useContext(UndoContext);
}

export function UndoProvider({ children }: { children: ReactNode }) {
  const toast = useToast();
  const announce = useAnnounce();
  const navigate = useNavigate();
  const location = useLocation();
  const locationRef = useRef(location);
  locationRef.current = location;
  const record = useRef<UndoRecord | null>(null);
  const pending = useRef<Promise<boolean> | null>(null);
  const [available, setAvailable] = useState(false);

  const undo = useCallback(async () => {
    if (pending.current !== null) {
      const ok = await pending.current;
      if (!ok) return;
    }
    const rec = record.current;
    if (rec === null) return;
    record.current = null;
    setAvailable(false);
    toast.dismiss(UNDO_KEY);
    try {
      if (rec.kind === "tag-remove") {
        const r = await addTags(
          rec.before.map((i) => i.id),
          [rec.tag],
        );
        const gone = new Set(r.notFound);
        const restored = rec.before.filter((i) => !gone.has(i.id));
        applyFeedback(restored, "undo");
        const sentence = `Restored tag ${rec.tag} on ${restored.length} ${restored.length === 1 ? "item" : "items"}.`;
        toast.show(sentence, "success", { key: UNDO_KEY });
        announce(sentence);
        return;
      }
      const gone = new Set<string>();
      for (const [status, ids] of groupByStatus(rec.before)) {
        const r = await bulkStatus(ids, status);
        for (const id of r.notFound) gone.add(id);
      }
      const restored = rec.before.filter((i) => !gone.has(i.id));
      applyFeedback(restored, "undo");
      const sentence = `Restored ${restored.length} ${restored.length === 1 ? "item" : "items"}.`;
      toast.show(sentence, "success", { key: UNDO_KEY });
      announce(sentence);
      const revisit = rec.revisit;
      if (revisit && locationRef.current.pathname === revisit.landedOn) {
        navigate(revisit.path, { replace: true, state: revisit.state });
      }
    } catch (e) {
      toast.show(errorMessage(e), "error");
    }
  }, [toast, announce, navigate]);

  const offer = useCallback(
    (rec: UndoRecord) => {
      record.current = rec;
      setAvailable(true);
      toast.show(rec.message, "success", {
        key: UNDO_KEY,
        duration: UNDO_DURATION,
        live: false,
        action: { label: "Undo", onAction: () => void undo() },
        // A newer offer replaces this toast; only the current record's own toast clears it.
        onClose: () => {
          if (record.current === rec) {
            record.current = null;
            setAvailable(false);
          }
        },
      });
    },
    [toast, undo],
  );

  const track = useCallback((p: Promise<boolean>) => {
    const tracked: Promise<boolean> = p
      .catch(() => false)
      .finally(() => {
        if (pending.current === tracked) pending.current = null;
      });
    pending.current = tracked;
    return tracked;
  }, []);

  const undoRef = useRef(undo);
  undoRef.current = undo;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target instanceof HTMLElement ? e.target : null;
      const action = shortcutFor(
        {
          key: e.key,
          ctrlKey: e.ctrlKey,
          metaKey: e.metaKey,
          altKey: e.altKey,
          target: t
            ? { tagName: t.tagName, isContentEditable: t.isContentEditable, type: t instanceof HTMLInputElement ? t.type : undefined }
            : null,
        },
        { dialogOpen: document.querySelector('[aria-modal="true"]') !== null },
      );
      if (action !== "undo") return;
      if (record.current === null && pending.current === null) return;
      void undoRef.current();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const value = useMemo<UndoApi>(() => ({ offer, undo, available, track }), [offer, undo, available, track]);
  return <UndoContext.Provider value={value}>{children}</UndoContext.Provider>;
}
