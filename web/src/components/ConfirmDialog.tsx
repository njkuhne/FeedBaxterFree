import { useEffect, useRef, useState, type ReactNode } from "react";
import { errorMessage } from "../lib/api";
import { Button } from "./Button";
import { Dialog } from "./Dialog";

interface ConfirmDialogProps {
  title: string;
  body: ReactNode;
  confirmLabel: string;
  busyLabel: string;
  /** Resolve to close the dialog; reject to show the error inside it. */
  onConfirm: () => Promise<void>;
  onClose: () => void;
  /** Destructive: alertdialog, brick confirm, focus starts on Cancel. */
  danger?: boolean;
  /** Extra content (fields) between the body and the buttons. */
  children?: ReactNode;
}

export function ConfirmDialog({
  title,
  body,
  confirmLabel,
  busyLabel,
  onConfirm,
  onClose,
  danger = false,
  children,
}: ConfirmDialogProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const cancel = useRef<HTMLButtonElement | null>(null);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const confirm = async () => {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
      if (alive.current) onClose();
    } catch (e) {
      if (alive.current) {
        setError(errorMessage(e));
        setBusy(false);
      }
    }
  };

  return (
    <Dialog
      title={title}
      role={danger ? "alertdialog" : "dialog"}
      description={body}
      onClose={onClose}
      busy={busy}
      initialFocusRef={danger ? cancel : undefined}
    >
      {children}
      {error ? (
        <p role="alert" className="dialog-error">
          {error}
        </p>
      ) : null}
      <div className="dialog-actions">
        <Button ref={cancel} variant="text" onClick={onClose} disabled={busy}>
          Cancel
        </Button>
        <Button
          variant={danger ? "danger-solid" : "primary"}
          loading={busy}
          loadingLabel={busyLabel}
          onClick={() => void confirm()}
        >
          {confirmLabel}
        </Button>
      </div>
    </Dialog>
  );
}
