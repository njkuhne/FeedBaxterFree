import { useEffect, useId, useRef, type KeyboardEvent, type ReactNode, type RefObject } from "react";
import { createPortal } from "react-dom";

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

let openCount = 0;

/** True while any dialog is open (the app root is inert). */
export function isDialogOpen(): boolean {
  return openCount > 0;
}

function appRoot(): HTMLElement | null {
  return document.getElementById("app-root");
}

interface DialogProps {
  title: string;
  /** alertdialog for destructive confirmations. */
  role?: "dialog" | "alertdialog";
  /** Body text, wired to aria-describedby. */
  description?: ReactNode;
  onClose: () => void;
  /** While busy, Escape does nothing. */
  busy?: boolean;
  /** Element that gets focus on open; defaults to the first focusable element. */
  initialFocusRef?: RefObject<HTMLElement | null>;
  children?: ReactNode;
}

export function Dialog({
  title,
  role = "dialog",
  description,
  onClose,
  busy = false,
  initialFocusRef,
  children,
}: DialogProps) {
  const titleId = useId();
  const descId = useId();
  const panel = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    openCount += 1;
    appRoot()?.setAttribute("inert", "");
    const target = initialFocusRef?.current ?? panel.current?.querySelector<HTMLElement>(FOCUSABLE) ?? panel.current;
    target?.focus();
    return () => {
      openCount -= 1;
      if (openCount === 0) appRoot()?.removeAttribute("inert");
      if (opener && opener.isConnected) opener.focus();
    };
  }, []);

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      if (!busy) onClose();
      return;
    }
    if (e.key !== "Tab" || !panel.current) return;
    const items = Array.from(panel.current.querySelectorAll<HTMLElement>(FOCUSABLE));
    const first = items[0];
    const last = items[items.length - 1];
    if (!first || !last) {
      e.preventDefault();
      return;
    }
    const active = document.activeElement;
    if (e.shiftKey && (active === first || active === panel.current)) {
      e.preventDefault();
      last.focus();
    } else if (!e.shiftKey && active === last) {
      e.preventDefault();
      first.focus();
    }
  };

  return createPortal(
    <div className="scrim">
      <div
        ref={panel}
        className="dialog"
        role={role}
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        onKeyDown={onKeyDown}
      >
        <h2 id={titleId} className="dialog-title">
          {title}
        </h2>
        {description ? (
          <div id={descId} className="dialog-body">
            {description}
          </div>
        ) : null}
        {children}
      </div>
    </div>,
    document.body,
  );
}
