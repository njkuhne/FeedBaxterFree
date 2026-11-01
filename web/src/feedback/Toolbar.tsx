import { useId, useRef, useState, type KeyboardEvent, type RefObject } from "react";
import { Button } from "../components/Button";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { ChevronDownIcon, SearchIcon, XIcon } from "../components/icons";
import { MAX_QUERY } from "../lib/filters";
import type { Channel, Filters, TagCount } from "../lib/types";
import { ExportMenu, ExportPanel } from "./ExportMenu";

interface ToolbarProps {
  filters: Filters;
  /** The search box shows this, which runs ahead of filters.q while the user types. */
  search: string;
  searchRef: RefObject<HTMLInputElement | null>;
  tags: TagCount[];
  channels: Channel[];
  /** Items per channel ID, from the unfiltered first load. */
  channelCounts: Record<string, number>;
  /** The active tab's count, or null while unknown. */
  count: number | null;
  onSearch: (q: string) => void;
  /** Clears the search at once (no pause) and keeps focus in the field. */
  onClearSearch: () => void;
  onTag: (tag: string) => void;
  onChannel: (channel: string) => void;
  /** Below 880 px: Tag, Channel and Export move behind a Filters button. */
  narrow?: boolean;
}

/** Search, Tag, Channel and Export on one line; on narrow screens search and a Filters button. */
export function Toolbar({
  filters,
  search,
  searchRef,
  tags,
  channels,
  channelCounts,
  count,
  onSearch,
  onClearSearch,
  onTag,
  onChannel,
  narrow = false,
}: ToolbarProps) {
  const [open, setOpen] = useState(false);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const panelId = useId();
  const tagKnown = filters.tag === "" || tags.some((t) => t.name === filters.tag);
  const channelKnown = filters.channel === "" || channels.some((c) => c.id === filters.channel);
  const channelName = channels.find((c) => c.id === filters.channel)?.name;
  const active = (filters.tag !== "" ? 1 : 0) + (filters.channel !== "" ? 1 : 0);

  const tagSelect = (hideLabel: boolean) => (
    <SelectField
      label="Tag"
      hideLabel={hideLabel}
      className="toolbar-select"
      value={filters.tag}
      onChange={(e) => onTag(e.target.value)}
    >
      <option value="">All tags</option>
      {tagKnown ? null : <option value={filters.tag}>{filters.tag}</option>}
      {tags.map((t) => (
        <option key={t.id} value={t.name}>
          {`${t.name} (${t.count})`}
        </option>
      ))}
    </SelectField>
  );
  const channelSelect = (hideLabel: boolean) =>
    channels.length >= 2 ? (
      <SelectField
        label="Channel"
        hideLabel={hideLabel}
        className="toolbar-select"
        value={filters.channel}
        onChange={(e) => onChannel(e.target.value)}
      >
        <option value="">All channels</option>
        {channelKnown ? null : <option value={filters.channel}>Unknown channel</option>}
        {channels.map((c) => (
          <option key={c.id} value={c.id}>
            {`${c.name} (${channelCounts[c.id] ?? 0})`}
          </option>
        ))}
      </SelectField>
    ) : null;

  // Escape inside the panel closes it (and only that: it must not also clear a list selection).
  const onPanelKey = (e: KeyboardEvent) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    setOpen(false);
    buttonRef.current?.focus();
  };

  return (
    <div className="toolbar" role="search" aria-label="Filter feedback">
      <div className="search-box">
        <SearchIcon className="search-icon" />
        <TextField
          label="Search"
          hideLabel
          type="search"
          placeholder="Search messages"
          ref={searchRef}
          value={search}
          maxLength={MAX_QUERY}
          onChange={(e) => onSearch(e.target.value)}
        />
        {search !== "" ? (
          <button
            type="button"
            className="search-clear"
            aria-label="Clear search"
            onClick={() => {
              onClearSearch();
              searchRef.current?.focus();
            }}
          >
            <XIcon />
          </button>
        ) : null}
      </div>
      {narrow ? (
        <>
          <Button
            variant="text"
            ref={buttonRef}
            className="filters-button"
            aria-expanded={open}
            aria-controls={panelId}
            onClick={() => setOpen((o) => !o)}
          >
            {active > 0 ? `Filters \u00b7 ${active}` : "Filters"}
            <ChevronDownIcon className={open ? "topbar-chevron topbar-chevron-open" : "topbar-chevron"} />
          </Button>
          <div id={panelId} className="filters-panel" hidden={!open} onKeyDown={onPanelKey}>
            {tagSelect(false)}
            {channelSelect(false)}
            <p className="filters-label">Export</p>
            <ExportPanel filters={filters} count={count} {...(channelName === undefined ? {} : { channelName })} />
          </div>
        </>
      ) : (
        <>
          {tagSelect(true)}
          {channelSelect(true)}
          <ExportMenu filters={filters} count={count} {...(channelName === undefined ? {} : { channelName })} />
        </>
      )}
    </div>
  );
}
