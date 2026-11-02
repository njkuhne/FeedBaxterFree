import type { ReactNode } from "react";
import { Button } from "../components/Button";
import { InlineAlert } from "../components/InlineAlert";
import { bytes, duration, plural } from "../lib/format";
import { AlertCircleIcon } from "../components/icons";
import { versionText } from "../lib/versionText";
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
            <Cell
              label="Could not be stored"
              value={num(ops.queue.dlq)}
              {...(ops.queue.dlq > 0 ? { icon: <AlertCircleIcon /> } : {})}
            />
            <Cell label="Accepted today" value={num(ops.counters.day)} />
            <Cell label="Accepted this month" value={num(ops.counters.month)} />
            <Cell label="Database size" value={bytes(ops.dbSizeBytes)} />
          </dl>
          {ops.queue.dlq > 0 ? (
            <p className="soft ops-note">
              {`${plural(ops.queue.dlq, "submission", "submissions")} could not be stored. They are kept aside; see "Signals" in OPS.md to inspect and replay them.`}
            </p>
          ) : null}
          {ops.queue.oldestAgeSeconds > 60 ? (
            <p className="soft ops-note">Processing is behind. Submissions are safe in the queue.</p>
          ) : null}
          <p className="muted ops-version">{versionText(ops)}</p>
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

function Cell({ label, value, icon }: { label: string; value: string; icon?: ReactNode }) {
  return (
    <div className="ops-cell">
      <dt>{label}</dt>
      <dd>
        {icon}
        {value}
      </dd>
    </div>
  );
}
