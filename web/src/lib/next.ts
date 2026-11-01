/** A post-login destination: same-origin paths only. Anything else becomes "/". */
export function safeNext(raw: string | null): string {
  if (raw === null || raw === "") return "/";
  if (!raw.startsWith("/") || raw.startsWith("//") || raw.startsWith("/\\")) return "/";
  return raw;
}

/** The login URL that returns to the given location afterwards. */
export function loginUrl(pathname: string, search: string): string {
  const here = pathname + search;
  return here === "/" || here === "" ? "/login" : `/login?next=${encodeURIComponent(here)}`;
}
