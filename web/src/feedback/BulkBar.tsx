import { useEffect } from "react";
import { Button } from "../components/Button";
import { Kbd } from "../components/Kbd";
import { usePrefs } from "../lib/prefs";
import type { Feedback, Status, TagCount } from "../lib/types";
import { usePopover } from "./ExportMenu";
import { ACTION_LABEL, ACTION_ORDER } from "./statusCopy";
import { TagEditor } from "./TagEditor";

interface BulkBarProps {
  count: number;
  /** Number of items in the running request, 0 when idle. */
  busy: number;
  /** The active tab: its own "Mark ..." action is left out. */
  status: Status | "all";
  selectedItems: Feedback[];
  /** Existing tags with counts, offered while typing. */
  tags: TagCount[];
  /** A "t" press asked for the tag popover. */
  tagRequested?: boolean;
  onTagOpened?: () => void;
  onStatus: (status: Status) => void;
  /** Resolves true when the tag was added. */
  onAddTag: (tag: string) => Promise<boolean>;
  onRemoveTag: (tag: string) => void;
  onClear: () => void;
  /** Narrow Select mode has its own Done button, so the bar leaves out Clear. */
  hideClear?: boolean;
  /** Narrow screens have no keyboard hints. */
  narrow?: boolean;
}

export const STATUS_KEY: Record<Status, string> = { reviewed: "e", actioned: "a", ignored: "i", new: "n" };

/** The popover behind "Tag…": the shared tag editor in add-only mode. It stays open for another tag. */
function BulkTagPopover({
  tags,
  disabled,
  requested,
  onOpened,
  onAddTag,
}: {
  tags: TagCount[];
  disabled: boolean;
  requested: boolean;
  onOpened?: (() => void) | undefined;
  onAddTag: (tag: string) => Promise<boolean>;
}) {
  const pop = usePopover();
  const { setOpen } = pop;

  // The "t" shortcut opens the popover from the list.
  useEffect(() => {
    if (!requested) return;
    setOpen(true);
    onOpened?.();
  }, [requested, setOpen, onOpened]);

  return (
    <div className="popover-wrap" ref={pop.wrapRef} onKeyDown={pop.onKeyDown}>
      <Button
        variant="text"
        ref={pop.buttonRef}
        disabled={disabled}
        aria-expanded={pop.open}
        aria-controls={pop.panelId}
        onClick={() => pop.setOpen((o) => !o)}
      >
        Tag…
      </Button>
      <div id={pop.panelId} className="popover" hidden={!pop.open}>
        {pop.open ? <TagEditor initiallyOpen suggestions={tags} onAdd={onAddTag} /> : null}
      </div>
    </div>
  );
}

function RemoveTagPopover({
  items,
  disabled,
  onRemove,
}: {
  items: Feedback[];
  disabled: boolean;
  onRemove: (tag: string) => void;
}) {
  const pop = usePopover();
  const counts = new Map<string, number>();
  for (const it of items) for (const t of it.tags) counts.set(t, (counts.get(t) ?? 0) + 1);
  const names = Array.from(counts.keys()).sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
  if (names.length === 0) return null;
  return (
    <div className="popover-wrap" ref={pop.wrapRef} onKeyDown={pop.onKeyDown}>
      <Button
        variant="text"
        ref={pop.buttonRef}
        disabled={disabled}
        aria-expanded={pop.open}
        aria-controls={pop.panelId}
        onClick={() => pop.setOpen((o) => !o)}
      >
        Remove tag…
      </Button>
      <div id={pop.panelId} className="popover" hidden={!pop.open}>
        <div className="popover-actions popover-stack">
          {names.map((n) => (
            <Button
              key={n}
              variant="text"
              onClick={() => {
                pop.close(true);
                onRemove(n);
              }}
            >
              {`${n} (${counts.get(n) ?? 0})`}
            </Button>
          ))}
        </div>
      </div>
    </div>
  );
}

/** The list header line while rows are selected. The select-all checkbox is the header's, not this bar's. */
export function BulkBar({
  count,
  busy,
  status,
  selectedItems,
  tags,
  tagRequested = false,
  onTagOpened,
  onStatus,
  onAddTag,
  onRemoveTag,
  onClear,
  hideClear = false,
  narrow = false,
}: BulkBarProps) {
  const working = busy > 0;
  const hints = usePrefs().singleKeys && !narrow;
  return (
    <div className="bulk" role="toolbar" aria-label="Bulk actions">
      <span className="bulk-count">{working ? `Updating ${busy}…` : `${count} selected`}</span>
      {ACTION_ORDER.filter((s) => s !== status).map((s) => (
        <Button
          key={s}
          variant="text"
          disabled={working}
          {...(hints ? { "aria-keyshortcuts": STATUS_KEY[s] } : {})}
          onClick={() => onStatus(s)}
        >
          {ACTION_LABEL[s]}
          {hints ? <Kbd k={STATUS_KEY[s]} /> : null}
        </Button>
      ))}
      <BulkTagPopover tags={tags} disabled={working} requested={tagRequested} onOpened={onTagOpened} onAddTag={onAddTag} />
      <RemoveTagPopover items={selectedItems} disabled={working} onRemove={onRemoveTag} />
      {hideClear ? null : (
        <Button variant="text" className="bulk-clear" disabled={working} onClick={onClear}>
          Clear
        </Button>
      )}
    </div>
  );
}
