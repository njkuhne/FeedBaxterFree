import { useEffect, useRef, type InputHTMLAttributes, type Ref } from "react";

interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  label: string;
  hideLabel?: boolean;
  indeterminate?: boolean;
  ref?: Ref<HTMLInputElement>;
}

export function Checkbox({ label, hideLabel, indeterminate = false, className, ref, ...rest }: CheckboxProps) {
  const own = useRef<HTMLInputElement | null>(null);
  useEffect(() => {
    if (own.current) own.current.indeterminate = indeterminate;
  }, [indeterminate]);
  const setRef = (el: HTMLInputElement | null) => {
    own.current = el;
    if (typeof ref === "function") ref(el);
    else if (ref) ref.current = el;
  };
  return (
    <label className={["checkbox", className].filter(Boolean).join(" ")}>
      <input {...rest} ref={setRef} type="checkbox" />
      <span className={hideLabel === true ? "visually-hidden" : "checkbox-label"}>{label}</span>
    </label>
  );
}
