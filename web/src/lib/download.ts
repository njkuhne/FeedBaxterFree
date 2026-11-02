/** The file name from a Content-Disposition header (`filename*` preferred), else the fallback. */
export function filenameFrom(contentDisposition: string | null, fallback: string): string {
  if (contentDisposition === null) return fallback;
  const star = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/i.exec(contentDisposition);
  if (star?.[1] !== undefined) {
    try {
      const name = decodeURIComponent(star[1].trim());
      if (name !== "") return name;
    } catch {
      // fall through to the plain form
    }
  }
  const plain = /filename\s*=\s*"([^"]*)"/i.exec(contentDisposition) ?? /filename\s*=\s*([^;\s]+)/i.exec(contentDisposition);
  const name = plain?.[1]?.trim();
  return name !== undefined && name !== "" ? name : fallback;
}

/** Fetches a file and saves it under the server's file name. Throws when the request fails. */
export async function downloadFile(url: string): Promise<void> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) throw new Error(`Download failed (${res.status}).`);
  const ext = /[?&]format=json\b/.test(url) ? "json" : "csv";
  const name = filenameFrom(res.headers.get("Content-Disposition"), `feedbaxter-export.${ext}`);
  const blob = await res.blob();
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = name;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(href), 0);
}
