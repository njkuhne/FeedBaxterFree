import { useId, type ReactNode, type Ref, type SelectHTMLAttributes } from "react";
import { FieldMessages } from "./TextField";
import { ChevronDownIcon } from "./icons";

interface SelectFieldProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, "id"> {
  label: string;
  hint?: string;
  error?: string | null;
  hideLabel?: boolean;
  children: ReactNode;
  ref?: Ref<HTMLSelectElement>;
}

export function SelectField({ label, hint, error, hideLabel, className, children, ref, ...rest }: SelectFieldProps) {
  const id = useId();
  const hintId = `${id}-hint`;
  const errorId = `${id}-error`;
  const describedBy = [hint ? hintId : "", error ? errorId : ""].filter(Boolean).join(" ");
  return (
    <div className={["field", className].filter(Boolean).join(" ")}>
      <label htmlFor={id} className={hideLabel === true ? "visually-hidden" : "field-label"}>
        {label}
      </label>
      <div className="select-wrap">
        <select
          {...rest}
          ref={ref}
          id={id}
          className="control control-select"
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy || undefined}
        >
          {children}
        </select>
        <ChevronDownIcon className="select-chevron" />
      </div>
      <FieldMessages hint={hint} hintId={hintId} error={error} errorId={errorId} />
    </div>
  );
}
