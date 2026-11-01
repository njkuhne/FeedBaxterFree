/** A v4 UUID: crypto.randomUUID, else getRandomValues, else a weaker Math.random mix. */
export function newId(): string {
  const c = (globalThis as { crypto?: Partial<Crypto> }).crypto;
  if (c && typeof c.randomUUID === "function") return c.randomUUID();
  const b = new Uint8Array(16);
  if (c && typeof c.getRandomValues === "function") {
    c.getRandomValues(b);
  } else {
    const t = Date.now();
    for (let i = 0; i < 16; i++) b[i] = Math.floor(Math.random() * 256) ^ ((t >>> ((i % 4) * 8)) & 255);
  }
  b[6] = ((b[6] ?? 0) & 15) | 64;
  b[8] = ((b[8] ?? 0) & 63) | 128;
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}
