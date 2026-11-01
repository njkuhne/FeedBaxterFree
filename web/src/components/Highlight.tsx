import { Fragment } from "react";

export interface Segment {
  text: string;
  match: boolean;
}

/**
 * Splits plain text into matching and non-matching pieces. Case-insensitive, every occurrence,
 * by `indexOf` on lowercased strings (no regular expression is built from the query).
 */
export function splitMatches(text: string, query: string): Segment[] {
  const q = query.trim().toLowerCase();
  const lower = text.toLowerCase();
  // Rare Unicode case mappings change the length; offsets would no longer line up.
  if (q === "" || lower.length !== text.length) return [{ text, match: false }];
  const out: Segment[] = [];
  let at = 0;
  for (;;) {
    const hit = lower.indexOf(q, at);
    if (hit < 0) break;
    if (hit > at) out.push({ text: text.slice(at, hit), match: false });
    out.push({ text: text.slice(hit, hit + q.length), match: true });
    at = hit + q.length;
  }
  if (at < text.length || out.length === 0) out.push({ text: text.slice(at), match: false });
  return out;
}

/** The text with every occurrence of the query in a <mark>. Only ever React text children, never HTML. */
export function Highlight({ text, query }: { text: string; query: string }) {
  return (
    <>
      {splitMatches(text, query).map((s, i) =>
        s.match ? (
          <mark key={i} className="hl">
            {s.text}
          </mark>
        ) : (
          <Fragment key={i}>{s.text}</Fragment>
        ),
      )}
    </>
  );
}
