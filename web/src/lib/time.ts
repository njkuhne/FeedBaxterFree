const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

const pad = (n: number) => String(n).padStart(2, "0");

/** "just now", "5 min ago", "2 h ago", "yesterday", "3 days ago", "1 Mar", "1 Mar 2025". Local time. */
export function relativeTime(iso: string, now: Date = new Date()): string {
  const then = new Date(iso);
  const seconds = (now.getTime() - then.getTime()) / 1000;
  if (seconds < 45) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${Math.max(minutes, 1)} min ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.floor(hours / 24);
  if (days < 2) return "yesterday";
  if (days < 7) return `${days} days ago`;
  const day = `${then.getDate()} ${MONTHS[then.getMonth()] ?? ""}`;
  return then.getFullYear() === now.getFullYear() ? day : `${day} ${then.getFullYear()}`;
}

/** "1 Mar 2026, 09:30" in local time. */
export function absoluteTime(iso: string): string {
  const d = new Date(iso);
  return `${dateOnly(iso)}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** "1 Mar 2026" in local time. */
export function dateOnly(iso: string): string {
  const d = new Date(iso);
  return `${d.getDate()} ${MONTHS[d.getMonth()] ?? ""} ${d.getFullYear()}`;
}

/** "10:31:05" in local time. */
export function clockTime(d: Date): string {
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** "1 Mar 2026" in local time, for lists and panels. */
export function shortDate(iso: string): string {
  return dateOnly(iso);
}
