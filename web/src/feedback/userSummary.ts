import { plural, platformName } from "../lib/format";
import { dateOnly } from "../lib/time";
import type { Feedback } from "../lib/types";

const MAX_VERSIONS = 3;

/** "28 Oct", with the year when it is not the current one. */
function day(iso: string, now: Date): string {
  const full = dateOnly(iso);
  return new Date(iso).getFullYear() === now.getFullYear() ? full.replace(/ \d{4}$/, "") : full;
}

function sameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function latest(iso: string, now: Date): string {
  const d = new Date(iso);
  if (sameDay(d, now)) return "today";
  const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
  return sameDay(d, yesterday) ? "yesterday" : day(iso, now);
}

function versionLabel(i: Feedback): string | null {
  const parts = [i.platform ? platformName(i.platform) : "", i.appVersion ?? ""].filter((p) => p !== "");
  return parts.length > 0 ? parts.join(" ") : null;
}

/**
 * One line under the per-user title: "2 items · first 28 Oct · latest today · Web 2.4.1, iOS 3.1.0".
 * `total` is the stats count when known; `complete` says every item is in `items` (so the oldest is known).
 */
export function userSummary(items: Feedback[], total: number | null, complete: boolean, now: Date): string {
  const sorted = [...items].sort((a, b) => (a.receivedAt < b.receivedAt ? 1 : a.receivedAt > b.receivedAt ? -1 : 0));
  const parts = [plural(total ?? items.length, "item", "items")];
  const newest = sorted[0];
  const oldest = sorted[sorted.length - 1];
  if (complete && oldest !== undefined) parts.push(`first ${day(oldest.receivedAt, now)}`);
  if (newest !== undefined) parts.push(`latest ${latest(newest.receivedAt, now)}`);
  const versions: string[] = [];
  for (const i of sorted) {
    const v = versionLabel(i);
    if (v !== null && !versions.includes(v)) versions.push(v);
  }
  if (versions.length > 0) {
    const shown = versions.slice(0, MAX_VERSIONS).join(", ");
    parts.push(versions.length > MAX_VERSIONS ? `${shown}, +${versions.length - MAX_VERSIONS} more` : shown);
  }
  return parts.join(" · ");
}
