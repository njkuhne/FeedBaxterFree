import { useCallback, useEffect, useId, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { Button } from "../components/Button";
import { STATUS_LABEL } from "../components/StatusMarker";
import { ChevronDownIcon, DownloadIcon } from "../components/icons";
import { downloadFile } from "../lib/download";
import { exportUrl } from "../lib/endpoints";
import { plural } from "../lib/format";
import type { Filters } from "../lib/types";

/**
 * A disclosure panel: opens below its button, closes on Escape (focus returns to the button), on a
 * click outside and on `close()`. Not a modal and not a menu. Shared by the export panel and the
 * bulk bar's tag popovers.
 */
export function usePopover() {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const panelId = useId();

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (e.target instanceof Node && wrapRef.current?.contains(e.target) !== true) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  const close = useCallback((returnFocus = false) => {
    setOpen(false);
    if (returnFocus) buttonRef.current?.focus();
  }, []);

  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (open && e.key === "Escape") {
      // Closing the panel is all Escape does here; it must not also clear a list selection.
      e.preventDefault();
      e.stopPropagation();
      close(true);
    }
  };

  return { open, setOpen, close, wrapRef, buttonRef, panelId, onKeyDown };
}

/** "Export 17 items · New, tag bug". The count is unknown while stats are loading. */
export function exportDescription(filters: Filters, count: number | null, channelName?: string): string {
  const head = count === null ? "Export all matching items" : `Export ${plural(count, "item", "items")}`;
  const parts: string[] = [];
  if (filters.status !== "all") parts.push(STATUS_LABEL[filters.status]);
  if (filters.tag !== "") parts.push(`tag ${filters.tag}`);
  if (filters.channel !== "") parts.push(`channel ${channelName ?? "unknown"}`);
  if (filters.q.trim() !== "") parts.push(`matching "${filters.q.trim()}"`);
  if (filters.userId !== "") parts.push(`from ${filters.userId}`);
  return parts.length === 0 ? head : `${head} · ${parts.join(", ")}`;
}

interface ExportMenuProps {
  filters: Filters;
  /** The active tab's count, or null while unknown. */
  count: number | null;
  channelName?: string;
}

interface ExportPanelProps extends ExportMenuProps {
  /** Called after an export succeeded (the popover closes itself). */
  onDone?: () => void;
}

/** What will be exported, and the CSV and JSON buttons. The popover and the narrow filters panel both show it. */
export function ExportPanel({ filters, count, channelName, onDone }: ExportPanelProps) {
  const [running, setRunning] = useState<"csv" | "json" | null>(null);
  const [failed, setFailed] = useState(false);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);

  const run = async (format: "csv" | "json") => {
    setFailed(false);
    setRunning(format);
    try {
      await downloadFile(exportUrl(filters, format));
      if (alive.current) setRunning(null);
      onDone?.();
    } catch {
      if (alive.current) {
        setRunning(null);
        setFailed(true);
      }
    }
  };

  return (
    <>
      <p className="popover-note">{exportDescription(filters, count, channelName)}</p>
      <div className="popover-actions">
        {(["csv", "json"] as const).map((format) => (
          <Button
            key={format}
            variant="text"
            loading={running === format}
            loadingLabel="Exporting…"
            disabled={running !== null}
            onClick={() => void run(format)}
          >
            {format.toUpperCase()}
          </Button>
        ))}
      </div>
      {failed ? (
        <p role="alert" className="form-error">
          Export failed. Try again.
        </p>
      ) : null}
    </>
  );
}

export function ExportMenu({ filters, count, channelName }: ExportMenuProps) {
  const pop = usePopover();
  return (
    <div className="popover-wrap" ref={pop.wrapRef} onKeyDown={pop.onKeyDown}>
      <Button
        variant="text"
        ref={pop.buttonRef}
        className="export-button"
        aria-expanded={pop.open}
        aria-controls={pop.panelId}
        onClick={() => pop.setOpen((o) => !o)}
      >
        <DownloadIcon />
        Export
        <ChevronDownIcon />
      </Button>
      <div id={pop.panelId} className="popover popover-end" hidden={!pop.open}>
        <ExportPanel filters={filters} count={count} onDone={() => pop.close()} {...(channelName === undefined ? {} : { channelName })} />
      </div>
    </div>
  );
}
