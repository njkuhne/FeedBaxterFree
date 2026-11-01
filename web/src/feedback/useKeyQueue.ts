import { useCallback, useEffect, useRef, useState } from "react";

export const KEY_QUEUE_MAX = 10;

export interface KeyQueue<T> {
  /** True while a request is running. */
  busy: boolean;
  /** Claims the in-flight slot; false when a request is already running. */
  begin: () => boolean;
  /** Releases the slot; queued keys then run one by one. */
  end: () => void;
  /** While a request runs, queues the item (dropping it beyond the cap) and returns true. Otherwise false. */
  offer: (item: T) => boolean;
}

/**
 * Buffers keypresses while a request is in flight and replays them in order afterwards,
 * each against the state the page has re-rendered with. `hold` pauses the replay.
 */
export function useKeyQueue<T>(run: (item: T) => void, hold = false): KeyQueue<T> {
  const [busy, setBusy] = useState(false);
  const [tick, setTick] = useState(0);
  const inflight = useRef(false);
  const queue = useRef<T[]>([]);
  const runRef = useRef(run);
  runRef.current = run;

  const begin = useCallback(() => {
    if (inflight.current) return false;
    inflight.current = true;
    setBusy(true);
    return true;
  }, []);
  const end = useCallback(() => {
    inflight.current = false;
    setBusy(false);
  }, []);
  const offer = useCallback((item: T) => {
    if (!inflight.current) return false;
    if (queue.current.length < KEY_QUEUE_MAX) queue.current.push(item);
    return true;
  }, []);

  useEffect(() => {
    if (busy || hold) return;
    const next = queue.current.shift();
    if (next === undefined) return;
    runRef.current(next);
    if (queue.current.length > 0) setTick((t) => t + 1);
  }, [busy, hold, tick]);

  return { busy, begin, end, offer };
}
