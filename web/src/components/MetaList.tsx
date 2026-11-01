import { createContext, useContext, type ReactNode } from "react";

const InList = createContext(true);

interface MetaListProps {
  className?: string;
  children: ReactNode;
  /** `ul` (the default) makes a real list. A row inside the inbox list uses `div` so it does not nest a second list in every row. */
  as?: "ul" | "div";
}

/**
 * A wrapping line of facts. The separators between items are drawn by CSS (see `.meta-list`), so no
 * dot is ever a text node and a wrapped line never starts or ends with one.
 */
export function MetaList({ className, children, as = "ul" }: MetaListProps) {
  const cls = ["meta-list", className].filter(Boolean).join(" ");
  return (
    <InList.Provider value={as === "ul"}>{as === "ul" ? <ul className={cls}>{children}</ul> : <div className={cls}>{children}</div>}</InList.Provider>
  );
}

/** One fact in a MetaList: an `li` in a list, a `span` otherwise. */
export function MetaItem({ className, children }: { className?: string; children: ReactNode }) {
  const inList = useContext(InList);
  return inList ? <li className={className}>{children}</li> : <span className={className}>{children}</span>;
}
