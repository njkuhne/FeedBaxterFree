import { useEffect, useRef } from "react";
import { useLocation } from "react-router-dom";
import { useNewCount } from "./NewCount";

let firstPage = true;

/**
 * Sets document.title to "<title> · FeedBaxter" and, on every navigation after
 * the first page load, moves focus to the page's h1. Call it from the page
 * component that renders the h1; pass null while the title is still unknown.
 */
export function usePageTitle(title: string | null): void {
  const { pathname: key } = useLocation(); // query-string changes (filters) must not steal focus
  const newCount = useNewCount();
  const prefix = newCount !== null && newCount > 0 ? `(${newCount}) ` : "";
  const focusedFor = useRef<string | null>(null);
  useEffect(() => {
    if (title === null) return;
    document.title = `${prefix}${title} · FeedBaxter`;
    if (focusedFor.current === key) return;
    focusedFor.current = key;
    if (firstPage) {
      firstPage = false;
      return;
    }
    document.querySelector<HTMLElement>("main h1, h1")?.focus();
  }, [title, key, prefix]);
}
