import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { usePolling } from "../admin/usePolling";
import { subscribe } from "../feedback/listStore";
import { stats } from "../lib/endpoints";

const POLL_MS = 30_000;

const NewCountContext = createContext<number | null>(null);

/**
 * The app-wide count of New feedback: fetched on mount, every 30 s while the tab is visible, and
 * again whenever the user changes feedback anywhere.
 */
export function NewCountProvider({ children }: { children: ReactNode }) {
  const [count, setCount] = useState<number | null>(null);
  const poll = useCallback(async () => {
    try {
      const s = await stats();
      setCount(s.byStatus.new ?? 0);
    } catch {
      // keep the last value; null stays null
    }
  }, []);
  const { refresh } = usePolling(poll, POLL_MS);
  useEffect(() => subscribe(() => refresh()), [refresh]);
  return <NewCountContext.Provider value={count}>{children}</NewCountContext.Provider>;
}

/** The New count, or null before the first answer (and outside the signed-in shell). */
export function useNewCount(): number | null {
  return useContext(NewCountContext);
}
