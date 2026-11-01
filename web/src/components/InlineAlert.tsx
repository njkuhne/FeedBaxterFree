import type { ReactNode } from "react";
import { Button } from "./Button";

interface InlineAlertProps {
  children: ReactNode;
  onRetry?: () => void;
}

export function InlineAlert({ children, onRetry }: InlineAlertProps) {
  return (
    <div className="inline-alert" role="alert">
      <span>{children}</span>
      {onRetry ? (
        <Button variant="text" onClick={onRetry}>
          Try again
        </Button>
      ) : null}
    </div>
  );
}
