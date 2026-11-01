import type { RefObject } from "react";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { SearchIcon, XIcon } from "../components/icons";
import { MAX_QUERY } from "../lib/filters";
import type { Channel, Filters, TagCount } from "../lib/types";
import { ExportMenu } from "./ExportMenu";

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
}

/** Search, Tag, Channel and Export on one line. */
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
}: ToolbarProps) {
  const tagKnown = filters.tag === "" || tags.some((t) => t.name === filters.tag);
  const channelKnown = filters.channel === "" || channels.some((c) => c.id === filters.channel);
  const channelName = channels.find((c) => c.id === filters.channel)?.name;
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
      <SelectField
        label="Tag"
        hideLabel
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
      {channels.length >= 2 ? (
        <SelectField
          label="Channel"
          hideLabel
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
      ) : null}
      <ExportMenu filters={filters} count={count} {...(channelName === undefined ? {} : { channelName })} />
    </div>
  );
}
