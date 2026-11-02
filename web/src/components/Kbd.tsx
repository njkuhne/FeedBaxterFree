/** The faint key hint beside a button label. Hidden from assistive technology: the button carries aria-keyshortcuts. */
export function Kbd({ k }: { k: string }) {
  return (
    <kbd className="kbd-hint" aria-hidden="true">
      {k}
    </kbd>
  );
}
