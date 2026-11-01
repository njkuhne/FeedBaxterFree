import type { ReactNode } from "react";

/** The page's single h1. Focusable so route changes can move focus to it. */
export function PageTitle({ children, hidden = false }: { children: ReactNode; hidden?: boolean }) {
  return (
    <h1 className={hidden ? "page-title visually-hidden" : "page-title"} tabIndex={-1}>
      {children}
    </h1>
  );
}
