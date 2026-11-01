import type { ReactNode, Ref } from "react";
import { BaxterFull } from "./Baxter";

interface EmptyStateProps {
  heading: string;
  body: string;
  action?: ReactNode;
  headingRef?: Ref<HTMLHeadingElement>;
  /** Baxter shows for inbox zero and onboarding only; filtered empties pass false. */
  mark?: boolean;
}

export function EmptyState({ heading, body, action, headingRef, mark = true }: EmptyStateProps) {
  return (
    <div className="empty">
      {mark ? <BaxterFull size={128} /> : null}
      <h2 ref={headingRef} tabIndex={-1} className="empty-heading">
        {heading}
      </h2>
      <p className="empty-body">{body}</p>
      {action}
    </div>
  );
}
