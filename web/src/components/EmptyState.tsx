import type { ReactNode, Ref } from "react";
import { BaxterFull } from "./Baxter";

interface EmptyStateProps {
  heading: string;
  body: string;
  action?: ReactNode;
  headingRef?: Ref<HTMLHeadingElement>;
}

export function EmptyState({ heading, body, action, headingRef }: EmptyStateProps) {
  return (
    <div className="empty">
      <BaxterFull size={128} />
      <h2 ref={headingRef} tabIndex={-1} className="empty-heading">
        {heading}
      </h2>
      <p className="empty-body">{body}</p>
      {action}
    </div>
  );
}
