import type { Status } from "../lib/types";
import { STATUS_LABEL, StatusMarker } from "./StatusMarker";

export function StatusChip({ status }: { status: Status }) {
  return (
    <span className={`chip chip-${status}`}>
      <StatusMarker status={status} />
      {STATUS_LABEL[status]}
    </span>
  );
}
