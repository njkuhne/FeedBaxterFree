import { STATUSES, type Feedback, type Filters, type Status } from "./types";

export const MAX_QUERY = 200;

export const defaultFilters: Filters = { status: "new", tag: "", channel: "", q: "", userId: "" };

function isStatus(s: string | null): s is Status {
  return s !== null && (STATUSES as readonly string[]).includes(s);
}

/** Read the inbox filters from the page query string. Unknown statuses fall back to the default. */
export function filtersFromQuery(params: URLSearchParams): Filters {
  const status = params.get("status");
  return {
    status: status === "all" ? "all" : isStatus(status) ? status : "new",
    tag: params.get("tag") ?? "",
    channel: params.get("channel") ?? "",
    q: params.get("q") ?? "",
    userId: "",
  };
}

/** The page query string for the filters. The default status (new) and empty values are omitted. */
export function filtersToQuery(f: Filters): URLSearchParams {
  const params = new URLSearchParams();
  if (f.status !== "new") params.set("status", f.status);
  if (f.tag !== "") params.set("tag", f.tag);
  if (f.channel !== "") params.set("channel", f.channel);
  if (f.q.trim() !== "") params.set("q", f.q);
  return params;
}

/** Query parameters for the API (list and export). "all" sends no status; empty values are dropped. */
export function toApiParams(f: Filters): URLSearchParams {
  const params = new URLSearchParams();
  if (f.status !== "all") params.set("status", f.status);
  if (f.tag !== "") params.set("tag", f.tag);
  if (f.channel !== "") params.set("channel", f.channel);
  if (f.q.trim() !== "") params.set("q", f.q.trim().slice(0, MAX_QUERY));
  if (f.userId !== "") params.set("userId", f.userId);
  return params;
}

/** Whether an item belongs in the list for these filters. Same semantics as the API. */
export function matchesFilters(item: Feedback, f: Filters): boolean {
  if (f.status !== "all" && item.status !== f.status) return false;
  if (f.tag !== "") {
    const tag = f.tag.toLowerCase();
    if (!item.tags.some((t) => t.toLowerCase() === tag)) return false;
  }
  if (f.channel !== "" && item.channelId !== f.channel) return false;
  if (f.userId !== "" && item.userId !== f.userId) return false;
  const q = f.q.trim().toLowerCase();
  if (q !== "" && !item.message.toLowerCase().includes(q)) return false;
  return true;
}
