import { forwardRef } from "react";

interface NewPillProps {
  /** How many items arrived since the list was loaded. */
  count: number;
  onShow: () => void;
}

/** "3 new · Show": a quiet button above the list. Pressing it reloads the list; rows never appear on their own. */
export const NewPill = forwardRef<HTMLButtonElement, NewPillProps>(function NewPill({ count, onShow }, ref) {
  return (
    <button type="button" ref={ref} className="new-pill" onClick={onShow}>
      {`${count} new · Show`}
    </button>
  );
});
