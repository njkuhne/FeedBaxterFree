import type { HistoryEntry } from "../lib/types";
import { dateOnly } from "../lib/time";

const pad = (n: number) => String(n).padStart(2, "0");

/** "1 Nov 2026 15:57" in local time. */
export function historyTime(iso: string): string {
  const d = new Date(iso);
  return `${dateOnly(iso)} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "Received 28 Oct 2026 09:12" */
export function receivedSentence(iso: string): string {
  return `Received ${historyTime(iso)}`;
}

/** The sentence before the time: "Marked reviewed by admin@localhost via MCP". */
export function historyLead(h: HistoryEntry): string {
  const who = h.userEmail ?? "a removed user";
  return `Marked ${h.to} by ${who}${h.via === "mcp" ? " via MCP" : ""}`;
}

/** "Marked reviewed by admin@localhost, 1 Nov 2026 15:57" */
export function historySentence(h: HistoryEntry): string {
  return `${historyLead(h)}, ${historyTime(h.at)}`;
}
