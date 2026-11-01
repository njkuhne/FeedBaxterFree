import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

type Announce = (message: string) => void;

const AnnounceContext = createContext<Announce>(() => undefined);

/** Say something to screen readers without a visible toast. */
export function useAnnounce(): Announce {
  return useContext(AnnounceContext);
}

export function AnnouncerProvider({ children }: { children: ReactNode }) {
  const [text, setText] = useState("");
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const announce = useCallback<Announce>((message) => {
    // Clear first so an identical message is announced again.
    setText("");
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setText(message), 50);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  const value = useMemo(() => announce, [announce]);
  return (
    <AnnounceContext.Provider value={value}>
      {children}
      {createPortal(
        <div className="visually-hidden" role="status" aria-live="polite" aria-atomic="true" data-testid="announcer">
          {text}
        </div>,
        document.body,
      )}
    </AnnounceContext.Provider>
  );
}
