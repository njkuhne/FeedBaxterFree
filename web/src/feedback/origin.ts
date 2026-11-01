import { STATUS_LABEL } from "../components/StatusMarker";
import { STATUSES, type Filters } from "../lib/types";

/** Where a detail page was opened from, carried in router state. */
export interface FromList {
  kind: "inbox" | "user";
  /** listKey(filters) */
  key: string;
  filters: Filters;
  /** pathname + search of the list, e.g. "/?status=reviewed&tag=bug" or "/user/usr_8f2a91" */
  path: string;
  /** "Inbox" or the user ID */
  label: string;
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null;
}

function isFilters(v: unknown): v is Filters {
  if (!isObject(v)) return false;
  const status = v["status"];
  return (
    (status === "all" || (typeof status === "string" && (STATUSES as readonly string[]).includes(status))) &&
    typeof v["tag"] === "string" &&
    typeof v["channel"] === "string" &&
    typeof v["q"] === "string" &&
    typeof v["userId"] === "string"
  );
}

/** Reads the origin out of router state; anything malformed is null. */
export function readFrom(state: unknown): FromList | null {
  if (!isObject(state)) return null;
  const from = state["from"];
  if (!isObject(from)) return null;
  const { kind, key, filters, path, label } = from;
  if (kind !== "inbox" && kind !== "user") return null;
  if (typeof key !== "string" || typeof path !== "string" || typeof label !== "string") return null;
  // Only in-app paths: never a protocol-relative or absolute URL.
  if (!path.startsWith("/") || path.startsWith("//")) return null;
  if (!isFilters(filters)) return null;
  return { kind, key, filters, path, label };
}

/** "in New", "in All", or "from usr_8f2a91". */
export function positionLabel(from: FromList): string {
  if (from.kind === "user") return `from ${from.label}`;
  return `in ${from.filters.status === "all" ? "All" : STATUS_LABEL[from.filters.status]}`;
}
