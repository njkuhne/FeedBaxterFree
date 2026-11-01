import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { XIcon } from "./icons";

export type ToastKind = "success" | "info" | "error";

interface ToastApi {
  show: (message: string, kind?: ToastKind) => void;
}

interface ToastItem {
  id: number;
  message: string;
  kind: ToastKind;
}

const MAX_TOASTS = 3;
const noop: ToastApi = { show: () => undefined };
const ToastContext = createContext<ToastApi>(noop);

export function useToast(): ToastApi {
  return useContext(ToastContext);
}

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const nextId = useRef(1);
  const show = useCallback((message: string, kind: ToastKind = "success") => {
    const id = nextId.current++;
    setItems((cur) => [...cur, { id, message, kind }].slice(-MAX_TOASTS));
  }, []);
  const dismiss = useCallback((id: number) => setItems((cur) => cur.filter((t) => t.id !== id)), []);
  const api = useMemo<ToastApi>(() => ({ show }), [show]);
  const polite = items.filter((t) => t.kind !== "error");
  const alerts = items.filter((t) => t.kind === "error");
  return (
    <ToastContext.Provider value={api}>
      {children}
      {createPortal(
        <div className="toasts">
          <div role="status" aria-live="polite" className="toast-region">
            {polite.map((t) => (
              <ToastView key={t.id} item={t} onDismiss={dismiss} />
            ))}
          </div>
          <div role="alert" className="toast-region">
            {alerts.map((t) => (
              <ToastView key={t.id} item={t} onDismiss={dismiss} />
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
  const remaining = useRef(item.kind === "error" ? 8000 : 5000);
  useEffect(() => {
    if (paused) return;
    const started = Date.now();
    const timer = setTimeout(() => onDismiss(item.id), remaining.current);
    return () => {
      clearTimeout(timer);
      remaining.current = Math.max(0, remaining.current - (Date.now() - started));
    };
  }, [paused, item.id, onDismiss]);
  return (
    <div
      className={`toast toast-${item.kind}`}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
    >
      <span className="toast-message">{item.message}</span>
      <button type="button" className="toast-dismiss" aria-label="Dismiss" onClick={() => onDismiss(item.id)}>
        <XIcon />
      </button>
    </div>
  );
}
