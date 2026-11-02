import { createContext, Fragment, useCallback, useContext, useEffect, useId, useMemo, useState, type ReactNode } from "react";
import { Button } from "../components/Button";
import { Checkbox } from "../components/Checkbox";
import { Dialog } from "../components/Dialog";
import { getPrefs, setPref, usePrefs } from "../lib/prefs";
import { shortcutFor } from "../lib/shortcuts";
import { useAuth } from "../shell/AuthProvider";

interface ShortcutsApi {
  open: () => void;
}

const ShortcutsContext = createContext<ShortcutsApi>({ open: () => undefined });

export function useShortcuts(): ShortcutsApi {
  return useContext(ShortcutsContext);
}

/** A row's keys: a string is plain text between keys, `{ k }` is one key. */
type Part = string | { k: string };
interface Row {
  keys: Part[];
  text: string;
}
interface Group {
  title: string;
  rows: Row[];
}

const k = (key: string): Part => ({ k: key });

const GROUPS: Group[] = [
  {
    title: "List",
    rows: [
      { keys: [k("j"), " or ", k("↓")], text: "Next item" },
      { keys: [k("k"), " or ", k("↑")], text: "Previous item" },
      { keys: [k("Enter")], text: "Open the item" },
      { keys: [k("/")], text: "Search" },
    ],
  },
  {
    title: "Status (list and detail)",
    rows: [
      { keys: [k("e"), " or ", k("r")], text: "Mark reviewed: read, still open" },
      { keys: [k("a")], text: "Mark actioned: something was done about it" },
      { keys: [k("i")], text: "Mark ignored" },
      { keys: [k("n")], text: "Mark new: reopen it" },
      { keys: [k("t")], text: "Add a tag" },
    ],
  },
  {
    title: "Detail",
    rows: [
      { keys: [k("j")], text: "Next item" },
      { keys: [k("k")], text: "Previous item" },
      { keys: [k("u"), " or ", k("Escape")], text: "Back to the list" },
    ],
  },
  {
    title: "Selection",
    rows: [
      { keys: [k("x")], text: "Select or deselect the item" },
      { keys: [k("Shift"), "+", k("x"), " or Shift+click"], text: "Select a range" },
      { keys: [k("Shift"), "+", k("j"), " / ", k("Shift"), "+", k("k")], text: "Select the next or previous item too" },
      { keys: [k("Escape")], text: "Clear the selection" },
    ],
  },
  { title: "Undo", rows: [{ keys: [k("z")], text: "Undo the last change, while its notice is showing" }] },
  { title: "Help", rows: [{ keys: [k("?")], text: "Show this list" }] },
];

function ShortcutsDialog({ onClose }: { onClose: () => void }) {
  const prefs = usePrefs();
  const advanceHint = useId();
  const keysHint = useId();
  return (
    <Dialog title="Keyboard shortcuts" className="dialog-wide" onClose={onClose}>
      <div className="shortcuts-body">
        {GROUPS.map((g) => (
          <section key={g.title} className="shortcut-group">
            <h3 className="shortcut-title">{g.title}</h3>
            <dl className="shortcut-list">
              {g.rows.map((r) => (
                <div key={r.text} className="shortcut-item">
                  <dt>
                    {r.keys.map((p, i) =>
                      typeof p === "string" ? <Fragment key={i}>{p}</Fragment> : <kbd key={i}>{p.k}</kbd>,
                    )}
                  </dt>
                  <dd>{r.text}</dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
        <section className="shortcut-group">
          <h3 className="shortcut-title">Settings</h3>
          <div className="shortcut-setting">
            <Checkbox
              role="switch"
              label="Advance to the next item after a decision"
              checked={prefs.advance}
              aria-describedby={advanceHint}
              onChange={(e) => setPref("advance", e.currentTarget.checked)}
            />
            <p id={advanceHint} className="muted">
              On the detail page, after you mark an item and it leaves the list.
            </p>
          </div>
          <div className="shortcut-setting">
            <Checkbox
              role="switch"
              label="Single-key shortcuts"
              checked={prefs.singleKeys}
              aria-describedby={keysHint}
              onChange={(e) => setPref("singleKeys", e.currentTarget.checked)}
            />
            <p id={keysHint} className="muted">
              Turn off if you use speech input or press keys by accident. Arrow keys, Enter, Escape and ? keep working.
            </p>
          </div>
        </section>
      </div>
      <div className="dialog-actions">
        <Button variant="text" onClick={onClose}>
          Close
        </Button>
      </div>
    </Dialog>
  );
}

/** `enabled` false: `?` does nothing (signed-out pages). */
export function ShortcutsProvider({ children, enabled = true }: { children: ReactNode; enabled?: boolean }) {
  const [shown, setShown] = useState(false);
  const open = useCallback(() => setShown(true), []);
  const close = useCallback(() => setShown(false), []);

  useEffect(() => {
    if (!enabled) {
      setShown(false);
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      const t = e.target instanceof HTMLElement ? e.target : null;
      const action = shortcutFor(
        {
          key: e.key,
          ctrlKey: e.ctrlKey,
          metaKey: e.metaKey,
          altKey: e.altKey,
          target: t
            ? { tagName: t.tagName, isContentEditable: t.isContentEditable, type: t instanceof HTMLInputElement ? t.type : undefined }
            : null,
        },
        { dialogOpen: document.querySelector('[aria-modal="true"]') !== null, singleKeys: getPrefs().singleKeys },
      );
      if (action !== "toggleHelp") return;
      e.preventDefault();
      setShown(true);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [enabled]);

  const value = useMemo<ShortcutsApi>(() => ({ open }), [open]);
  return (
    <ShortcutsContext.Provider value={value}>
      {children}
      {shown ? <ShortcutsDialog onClose={close} /> : null}
    </ShortcutsContext.Provider>
  );
}

/** The app's provider: the dialog and `?` exist only for a signed-in user who is not forced to change password. */
export function AppShortcuts({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  return <ShortcutsProvider enabled={user !== null && !user.mustChangePassword}>{children}</ShortcutsProvider>;
}
