import { Button } from "../components/Button";
import { InlineAlert } from "../components/InlineAlert";
import { bytes, duration } from "../lib/format";
import { clockTime } from "../lib/time";
import type { Ops } from "../lib/types";

interface OpsGridProps {
  /** Latest successful reading, or null before the first one. */
  ops: Ops | null;
  /** The last request failed. */
  unavailable: boolean;
  updatedAt: Date | null;
  onRefresh: () => void;
}

const num = (n: number) => n.toLocaleString("en-GB");

export function OpsGrid({ ops, unavailable, updatedAt, onRefresh }: OpsGridProps) {
  return (
    <>
      {unavailable ? (
        <InlineAlert>Operations data is unavailable right now. Retrying every 10 seconds.</InlineAlert>
      ) : ops === null ? (
        <p className="muted">Loading…</p>
      ) : (
        <>
          <dl className="ops-grid">
            <Cell label="Waiting in queue" value={num(ops.queue.depth)} />
            <Cell
              label="Oldest waiting"
              value={ops.queue.oldestAgeSeconds === 0 ? "None" : duration(ops.queue.oldestAgeSeconds)}
            />
            <Cell label="Could not be stored" value={num(ops.queue.dlq)} />
            <Cell label="Accepted today" value={num(ops.counters.day)} />
            <Cell label="Accepted this month" value={num(ops.counters.month)} />
            <Cell label="Database size" value={bytes(ops.dbSizeBytes)} />
            <Cell
              label="Version"
              value={`${ops.version} · ${ops.mode} mode · ${ops.dialect === "postgres" ? "Postgres" : "SQLite"}`}
            />
          </dl>
          {ops.queue.dlq > 0 ? (
            <p className="soft ops-note">Some submissions could not be stored. See "Signals" in OPS.md.</p>
          ) : null}
          {ops.queue.oldestAgeSeconds > 60 ? (
            <p className="soft ops-note">Processing is behind. Submissions are safe in the queue.</p>
          ) : null}
        </>
      )}
      <p className="muted ops-footer">
        {updatedAt ? `Updated ${clockTime(updatedAt)}. ` : ""}Refreshes every 10 seconds.
        <Button variant="text" onClick={onRefresh}>
          Refresh now
        </Button>
      </p>
    </>
  );
}

function Cell({ label, value }: { label: string; value: string }) {
  return (
    <div className="ops-cell">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}
