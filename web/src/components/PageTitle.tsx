import type { ReactNode } from "react";

/** The page's single h1. Focusable so route changes can move focus to it. */
export function PageTitle({ children }: { children: ReactNode }) {
  return (
    <h1 className="page-title" tabIndex={-1}>
      {children}
    </h1>
  );
}
