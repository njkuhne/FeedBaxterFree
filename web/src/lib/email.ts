const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** True when the trimmed string looks like an email address. */
export function looksLikeEmail(s: string): boolean {
  return EMAIL.test(s.trim());
}
