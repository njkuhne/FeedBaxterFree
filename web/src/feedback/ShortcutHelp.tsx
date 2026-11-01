import { useId } from "react";
import { Button } from "../components/Button";

const SHORTCUTS: [string, string][] = [
  ["j or Down", "Next row"],
  ["k or Up", "Previous row"],
  ["x", "Select or deselect the row"],
  ["Enter", "Open the row"],
  ["e", "Mark reviewed"],
  ["a", "Mark actioned"],
  ["i", "Ignore"],
  ["n", "Mark as new"],
  ["Escape", "Clear the selection"],
  ["/", "Search"],
  ["?", "Show or hide this list"],
];

interface ShortcutHelpProps {
  open: boolean;
  onToggle: () => void;
}

export function ShortcutHelp({ open, onToggle }: ShortcutHelpProps) {
  const id = useId();
  return (
    <div className="shortcuts">
      <Button variant="text" aria-expanded={open} aria-controls={id} onClick={onToggle}>
        Keyboard shortcuts
      </Button>
      {open ? (
        <dl id={id} className="facts shortcut-list">
          {SHORTCUTS.map(([key, text]) => (
            <div key={key} className="shortcut-item">
              <dt>
                <kbd>{key}</kbd>
              </dt>
              <dd>{text}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </div>
  );
}
