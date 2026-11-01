import { useCallback, useEffect, useRef } from "react";

/**
 * Runs fn now, then every intervalMs. Pauses while the page is hidden and runs
 * once when it becomes visible again. Never overlaps: a run is skipped while the
 * previous one is still in flight. Returns refresh(), which runs now and
 * restarts the timer.
 */
export function usePolling(fn: () => Promise<void>, intervalMs: number): { refresh: () => void } {
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const runRef = useRef<() => void>(() => undefined);

  useEffect(() => {
    let alive = true;
    let inFlight = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const clear = () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
    };
    const schedule = () => {
      clear();
      if (alive && document.visibilityState !== "hidden") timer = setTimeout(run, intervalMs);
    };
    function run() {
      clear();
      if (!alive || inFlight) return;
      inFlight = true;
      fnRef
        .current()
        .catch(() => undefined)
        .finally(() => {
          inFlight = false;
          schedule();
        });
    }
    const onVisibility = () => {
      if (document.visibilityState === "hidden") clear();
      else run();
    };

    runRef.current = run;
    document.addEventListener("visibilitychange", onVisibility);
    run();
    return () => {
      alive = false;
      clear();
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [intervalMs]);

  const refresh = useCallback(() => runRef.current(), []);
  return { refresh };
}
