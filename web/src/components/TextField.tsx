import { useId, type InputHTMLAttributes, type ReactNode, type Ref } from "react";
import { AlertCircleIcon } from "./icons";

interface TextFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "id"> {
  label: string;
  hint?: string;
  error?: string | null;
  /** Keep the label for assistive technology but hide it visually. */
  hideLabel?: boolean;
  ref?: Ref<HTMLInputElement>;
}

export function TextField({ label, hint, error, hideLabel, className, ref, ...rest }: TextFieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [hint ? hintId : "", error ? errorId : ""].filter(Boolean).join(" ");
  return (
    <div className={["field", className].filter(Boolean).join(" ")}>
      <label htmlFor={id} className={hideLabel === true ? "visually-hidden" : "field-label"}>
        {label}
      </label>
      <input
        {...rest}
        ref={ref}
        id={id}
        className="control"
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy || undefined}
      />
      <FieldMessages hint={hint} hintId={hintId} error={error} errorId={errorId} />
    </div>
  );
}

export function FieldMessages({
  hint,
  hintId,
  error,
  errorId,
}: {
  hint?: string | undefined;
  hintId: string;
  error?: string | null | undefined;
  errorId: string;
}): ReactNode {
  return (
    <>
      {hint ? (
        <p id={hintId} className="field-hint">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="field-error">
          <AlertCircleIcon />
          <span>{error}</span>
        </p>
      ) : null}
    </>
  );
}
