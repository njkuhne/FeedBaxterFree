import { useEffect, useRef } from "react";
import { Button } from "./Button";
import { useCopy } from "./CodeBlock";

interface SecretOnceProps {
  title: string;
  value: string;
  note: string;
  onDone: () => void;
}

export function SecretOnce({ title, value, note, onDone }: SecretOnceProps) {
  const region = useRef<HTMLDivElement | null>(null);
  const { copied, copy } = useCopy();
  useEffect(() => {
    region.current?.focus();
  }, []);
  return (
    <div ref={region} className="secret" role="region" aria-label={title} tabIndex={-1}>
      <h3 className="secret-title">{title}</h3>
      <div className="secret-row">
        <input
          className="secret-value"
          readOnly
          value={value}
          aria-label="Secret value"
          onFocus={(e) => e.currentTarget.select()}
        />
        <Button variant="text" onClick={() => void copy(value)}>
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <p className="secret-note">{note}</p>
      <Button variant="text" onClick={onDone}>
        Done
      </Button>
    </div>
  );
}
