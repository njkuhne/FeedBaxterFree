import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { XIcon } from "./icons";

export type ToastKind = "success" | "info" | "error";

export interface ToastOptions {
  action?: { label: string; onAction: () => void };
  /** A toast with a key replaces any visible toast with the same key. */
  key?: string;
  /** Milliseconds; default 5000 (success, info), 8000 (error). */
  duration?: number;
  /** Called once when the toast leaves for any reason (timeout, dismiss, replaced, action). */
  onClose?: () => void;
  /** false: render outside the live regions because the caller announces it. Default true. */
  live?: boolean;
}

interface ToastApi {
  show: (message: string, kind?: ToastKind, options?: ToastOptions) => void;
  dismiss: (key: string) => void;
}

interface ToastItem {
  id: number;
  message: string;
  kind: ToastKind;
  key: string | undefined;
  action: ToastOptions["action"];
  duration: number;
  onClose: (() => void) | undefined;
  live: boolean;
}

const MAX_TOASTS = 3;
const noop: ToastApi = { show: () => undefined, dismiss: () => undefined };
const ToastContext = createContext<ToastApi>(noop);

export function useToast(): ToastApi {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const current = useRef<ToastItem[]>([]);
  const nextId = useRef(1);

  // The ref is the source of truth so onClose callbacks run once, outside any state updater.
  const commit = useCallback((next: ToastItem[], closed: ToastItem[]) => {
    current.current = next;
    setItems(next);
    for (const t of closed) t.onClose?.();
  }, []);

  const show = useCallback(
    (message: string, kind: ToastKind = "success", options: ToastOptions = {}) => {
      const item: ToastItem = {
        id: nextId.current++,
        message,
        kind,
        key: options.key,
        action: options.action,
        duration: options.duration ?? (kind === "error" ? 8000 : 5000),
        onClose: options.onClose,
        live: options.live ?? true,
      };
      const replaced = options.key === undefined ? [] : current.current.filter((t) => t.key === options.key);
      const kept = current.current.filter((t) => !replaced.includes(t));
      const all = [...kept, item];
      const overflow = all.slice(0, Math.max(0, all.length - MAX_TOASTS));
      commit(all.slice(-MAX_TOASTS), [...replaced, ...overflow]);
    },
    [commit],
  );
  const close = useCallback(
    (id: number) => {
      const gone = current.current.find((t) => t.id === id);
      if (gone === undefined) return;
      commit(
        current.current.filter((t) => t.id !== id),
        [gone],
      );
    },
    [commit],
  );
  const dismiss = useCallback(
    (key: string) => {
      const gone = current.current.filter((t) => t.key === key);
      if (gone.length === 0) return;
      commit(
        current.current.filter((t) => t.key !== key),
        gone,
      );
    },
    [commit],
  );
  const api = useMemo<ToastApi>(() => ({ show, dismiss }), [show, dismiss]);
  const polite = items.filter((t) => t.live && t.kind !== "error");
  const alerts = items.filter((t) => t.live && t.kind === "error");
  const silent = items.filter((t) => !t.live);
  return (
    <ToastContext.Provider value={api}>
      {children}
      {createPortal(
        <div className="toasts">
          <div className="toast-region">
            {silent.map((t) => (
              <ToastView key={t.id} item={t} onDismiss={close} />
            ))}
          </div>
          <div role="status" aria-live="polite" className="toast-region">
            {polite.map((t) => (
              <ToastView key={t.id} item={t} onDismiss={close} />
            ))}
          </div>
          <div role="alert" className="toast-region">
            {alerts.map((t) => (
              <ToastView key={t.id} item={t} onDismiss={close} />
            ))}
          </div>
        </div>,
        document.body,
      )}
    </ToastContext.Provider>
  );
}

function ToastView({ item, onDismiss }: { item: ToastItem; onDismiss: (id: number) => void }) {
  const [paused, setPaused] = useState(false);
  const remaining = useRef(item.duration);
  useEffect(() => {
    if (paused) return;
    const started = Date.now();
    const timer = setTimeout(() => onDismiss(item.id), remaining.current);
    return () => {
      clearTimeout(timer);
      remaining.current = Math.max(0, remaining.current - (Date.now() - started));
    };
  }, [paused, item.id, onDismiss]);
  const { action } = item;
  return (
    <div
      className={`toast toast-${item.kind}`}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <span className="toast-message">{item.message}</span>
      {action ? (
        <button
          type="button"
          className="toast-action"
          onClick={() => {
            action.onAction();
            onDismiss(item.id);
          }}
        >
          {action.label}
        </button>
      ) : null}
      <button type="button" className="toast-dismiss" aria-label="Dismiss" onClick={() => onDismiss(item.id)}>
        <XIcon />
      </button>
    </div>
  );
}
