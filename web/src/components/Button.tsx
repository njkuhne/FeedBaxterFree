import { useLayoutEffect, useRef, type ButtonHTMLAttributes, type Ref } from "react";

export type ButtonVariant = "primary" | "text" | "danger" | "danger-solid";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  /** Shows loadingLabel, disables the button and sets aria-busy. */
  loading?: boolean;
  loadingLabel?: string;
  ref?: Ref<HTMLButtonElement>;
}

export function Button({
  variant = "text",
  loading = false,
  loadingLabel,
  children,
  className,
  disabled,
  type = "button",
  style,
  ref,
  ...rest
}: ButtonProps) {
  const own = useRef<HTMLButtonElement | null>(null);
  const width = useRef<number | undefined>(undefined);
  useLayoutEffect(() => {
    if (!loading && own.current) width.current = own.current.offsetWidth;
  });
  const setRef = (el: HTMLButtonElement | null) => {
    own.current = el;
    if (typeof ref === "function") ref(el);
    else if (ref) ref.current = el;
  };
  const cls = ["btn", `btn-${variant}`, className].filter(Boolean).join(" ");
  const locked = loading && width.current ? { ...style, minWidth: width.current } : style;
  return (
    <button
      {...rest}
      ref={setRef}
      type={type}
      className={cls}
      style={locked}
      disabled={disabled === true || loading}
      aria-busy={loading ? true : undefined}
    >
      {loading && loadingLabel !== undefined ? loadingLabel : children}
    </button>
  );
}
