const UNITS = ["B", "KB", "MB", "GB", "TB"];

/** Binary units, one decimal above bytes: 1048576 -> "1.0 MB". */
export function bytes(n: number): string {
  if (n < 1024) return `${Math.max(0, Math.floor(n))} B`;
  let value = n;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(1)} ${UNITS[unit] ?? "B"}`;
}

/** Seconds as "45 s", "2 min 5 s", "1 h 3 min". */
export function duration(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds));
  if (total < 60) return `${total} s`;
  if (total < 3600) {
    const m = Math.floor(total / 60);
    const s = total % 60;
    return s === 0 ? `${m} min` : `${m} min ${s} s`;
  }
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  return m === 0 ? `${h} h` : `${h} h ${m} min`;
}

/** plural(1, "item", "items") -> "1 item". */
export function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}
