import { useEffect, useRef, useState } from "react";
import { copyText } from "../lib/clipboard";
import { useAnnounce } from "./Announcer";
import { Button } from "./Button";
import { useToast } from "./Toast";

interface CodeBlockProps {
  code: string;
  label: string;
}

/** Copies text, shows "Copied" for 2 s and announces it; on failure shows an error toast. */
export function useCopy(): { copied: boolean; copy: (text: string) => Promise<void> } {
  const [copied, setCopied] = useState(false);
  const announce = useAnnounce();
  const toast = useToast();
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const copy = async (text: string) => {
    if (await copyText(text)) {
      setCopied(true);
      announce("Copied to clipboard.");
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 2000);
    } else {
      toast.show("Could not copy. Select the text and copy it manually.", "error");
    }
  };
  return { copied, copy };
}

export function CodeBlock({ code, label }: CodeBlockProps) {
  const { copied, copy } = useCopy();
  return (
    <figure className="code">
      <figcaption className="code-caption">
        <span>{label}</span>
        <Button variant="text" onClick={() => void copy(code)}>
          {copied ? "Copied" : "Copy"}
        </Button>
      </figcaption>
      <pre tabIndex={0} aria-label={label}>
        <code>{code}</code>
      </pre>
    </figure>
  );
}
