import type { Status } from "../lib/types";

/** Text actions for each target status. */
export const ACTION_LABEL: Record<Status, string> = {
  reviewed: "Mark reviewed",
  actioned: "Mark actioned",
  ignored: "Mark ignored",
  new: "Mark new",
};

/** The order the actions are listed in. */
export const ACTION_ORDER: readonly Status[] = ["reviewed", "actioned", "ignored", "new"];

/** "Marked 3 as reviewed." plus " 1 was no longer there." when some ids were gone. */
export function markedSentence(updated: number, status: Status, notFound = 0): string {
  const base = `Marked ${updated} as ${status}.`;
  if (notFound <= 0) return base;
  return `${base} ${notFound} ${notFound === 1 ? "was" : "were"} no longer there.`;
}

/** Toast text after a status change: "Marked 3 reviewed". */
export function markedToast(count: number, status: Status): string {
  return `Marked ${count} ${status}`;
}

/** Appended to the screen reader announcement of a status change. */
export function withUndoHint(sentence: string): string {
  return `${sentence} Press z to undo.`;
}

/** Detail page: "Marked as reviewed." */
export function markedOneSentence(status: Status): string {
  return `Marked as ${status}.`;
}

export function addedTagSentence(tag: string, count: number): string {
  return `Added tag ${tag} to ${count}.`;
}

/** Tag names are 1 to 64 characters after trimming. */
export const TAG_ERROR = "Tags are 1 to 64 characters.";
export function validTag(raw: string): string | null {
  const t = raw.trim();
  return t.length >= 1 && t.length <= 64 ? t : null;
}
