import { useEffect, useRef } from "react";
import { Button } from "../components/Button";
import { Checkbox } from "../components/Checkbox";
import { XIcon } from "../components/icons";
import { plural } from "../lib/format";
import type { Feedback, Status, TagCount } from "../lib/types";
import { BulkBar } from "./BulkBar";

export interface FilterChip {
  key: string;
  /** "tag bug", "channel Web", "“safari”" */
  label: string;
  onRemove: () => void;
}

/** The line under the header while every loaded row is selected, or after "select all matching". */
export type SelectNotice =
  | { kind: "offer"; loaded: number; action: string; busy: boolean; onSelect: () => void }
  | { kind: "done"; text: string; onClear: () => void };

interface ListHeaderProps {
  /** The active tab's count, or null when unknown. */
  total: number | null;
  loaded: number;
  /** "Searching…" or "Loading…" while a new query is on its way (shown after a short delay), else null. */
  pending?: string | null;
  chips: FilterChip[];
  onClearAll: () => void;
  status: Status | "all";
  allSelected: boolean;
  selectedItems: Feedback[];
  selectedCount: number;
  busy: number;
  tags: TagCount[];
  /** A "t" press: the bulk bar opens its tag popover, then calls `onTagOpened`. */
  tagRequested?: boolean;
  onTagOpened?: () => void;
  notice: SelectNotice | null;
  onToggleAll: () => void;
  onStatus: (status: Status) => void;
  onAddTag: (tag: string) => Promise<boolean>;
  onRemoveTag: (tag: string) => void;
  onClear: () => void;
  /** Below 880 px the checkboxes and bulk bar live behind a Select button. */
  narrow?: boolean;
  selectMode?: boolean;
  onSelectMode?: (on: boolean) => void;
}

/**
 * One fixed-height line above the rows. It shows the summary and filter chips, or the bulk bar
 * when rows are selected, in the same place, with the select-all checkbox always first.
 */
export function ListHeader({
  total,
  loaded,
  pending = null,
  chips,
  onClearAll,
  status,
  allSelected,
  selectedItems,
  selectedCount,
  busy,
  tags,
  tagRequested = false,
  onTagOpened,
  notice,
  onToggleAll,
  onStatus,
  onAddTag,
  onRemoveTag,
  onClear,
  narrow = false,
  selectMode = false,
  onSelectMode,
}: ListHeaderProps) {
  const selecting = selectedCount > 0;
  const selectRef = useRef<HTMLButtonElement | null>(null);
  const leaving = useRef(false);
  // After Done, focus goes to the Select button that takes its place.
  useEffect(() => {
    if (!selectMode && leaving.current) {
      leaving.current = false;
      selectRef.current?.focus();
    }
  }, [selectMode]);
  // On narrow screens the checkbox and the bulk bar exist only in Select mode.
  const checks = !narrow || selectMode;
  const bulk = selecting && checks;
  return (
    <>
      <div className={narrow && bulk ? "list-head list-head-bulk" : "list-head"}>
        {checks ? (
          <Checkbox
            label="Select all on this page"
            hideLabel
            checked={allSelected}
            indeterminate={selecting && !allSelected}
            onChange={onToggleAll}
          />
        ) : null}
        {bulk ? (
          <BulkBar
            count={selectedCount}
            busy={busy}
            status={status}
            selectedItems={selectedItems}
            tags={tags}
            tagRequested={tagRequested}
            {...(onTagOpened ? { onTagOpened } : {})}
            onStatus={onStatus}
            onAddTag={onAddTag}
            onRemoveTag={onRemoveTag}
            onClear={onClear}
            hideClear={narrow}
            narrow={narrow}
          />
        ) : narrow && selectMode ? (
          <span className="list-hint">Select items</span>
        ) : (
          <div className="list-summary">
            <span className="list-count">{pending ?? (total === null ? `${loaded} loaded` : plural(total, "item", "items"))}</span>
            {chips.map((c) => (
              <span key={c.key} className="filter-chip">
                {c.label}
                <button type="button" className="filter-chip-remove" aria-label={`Remove filter ${c.label}`} onClick={c.onRemove}>
                  <XIcon />
                </button>
              </span>
            ))}
            {chips.length > 0 ? (
              <Button variant="text" onClick={onClearAll}>
                Clear all
              </Button>
            ) : null}
          </div>
        )}
        {narrow && !selectMode ? (
          <Button variant="text" ref={selectRef} className="list-select" onClick={() => onSelectMode?.(true)}>
            Select
          </Button>
        ) : null}
        {narrow && selectMode ? (
          <Button
            variant="text"
            className="list-select"
            onClick={() => {
              leaving.current = true;
              onSelectMode?.(false);
            }}
          >
            Done
          </Button>
        ) : null}
      </div>
      {notice !== null ? (
        <p className="select-notice">
          {notice.kind === "offer" ? (
            <>
              {`All ${notice.loaded} on this page are selected. `}
              <Button
                variant="text"
                loading={notice.busy}
                loadingLabel="Selecting…"
                onClick={notice.onSelect}
              >
                {notice.action}
              </Button>
            </>
          ) : (
            <>
              {`${notice.text} `}
              <Button variant="text" onClick={notice.onClear}>
                Clear selection
              </Button>
            </>
          )}
        </p>
      ) : null}
    </>
  );
}
