import { STATUS_LABEL } from "../components/StatusMarker";
import type { Filters } from "../lib/types";

/** Heading and body for a list that came back empty. New with no other filter is inbox zero and has its own copy. */
export function emptyCopy(filters: Filters, channelName?: string): { heading: string; body: string } {
  const q = filters.q.trim();
  const other = filters.tag !== "" || filters.channel !== "" || q !== "";
  const { status } = filters;
  if (!other) {
    if (status === "new") return { heading: "No new feedback", body: "New feedback appears here as it arrives." };
    if (status === "all") return { heading: "No feedback", body: "Feedback appears here as it arrives." };
    return {
      heading: `No ${STATUS_LABEL[status].toLowerCase()} feedback`,
      body: `Feedback you mark ${status} appears here.`,
    };
  }
  const parts: string[] = [];
  if (status !== "all") parts.push(`in ${STATUS_LABEL[status]}`);
  if (filters.tag !== "") parts.push(`tagged ${filters.tag}`);
  if (filters.channel !== "") parts.push(`in channel ${channelName ?? "unknown"}`);
  if (q !== "") parts.push(`matching "${q}"`);
  return { heading: "No matches", body: `Nothing ${parts.join(" ")}.` };
}
