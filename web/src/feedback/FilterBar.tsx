import type { Ref } from "react";
import { Segmented } from "../components/Segmented";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { MAX_QUERY } from "../lib/filters";
import type { Channel, Filters, Stats, Status, TagCount } from "../lib/types";

interface FilterBarProps {
  filters: Filters;
  stats: Stats;
  tags: TagCount[];
  channels: Channel[];
  /** The search box shows this, which runs ahead of filters.q while the user types. */
  search: string;
  searchRef: Ref<HTMLInputElement>;
  onStatus: (status: Status | "all") => void;
  onTag: (tag: string) => void;
  onChannel: (channel: string) => void;
  onSearch: (q: string) => void;
}

export function FilterBar({
  filters,
  stats,
  tags,
  channels,
  search,
  searchRef,
  onStatus,
  onTag,
  onChannel,
  onSearch,
}: FilterBarProps) {
  const count = (s: Status) => stats.byStatus[s] ?? 0;
  const tagKnown = filters.tag === "" || tags.some((t) => t.name === filters.tag);
  const channelKnown = filters.channel === "" || channels.some((c) => c.id === filters.channel);
  return (
    <div className="filters">
      <Segmented<Status | "all">
        name="status"
        label="Status"
        value={filters.status}
        onChange={onStatus}
        options={[
          { value: "new", label: "New", count: count("new") },
          { value: "reviewed", label: "Reviewed", count: count("reviewed") },
          { value: "actioned", label: "Actioned", count: count("actioned") },
          { value: "ignored", label: "Ignored", count: count("ignored") },
          { value: "all", label: "All", count: stats.total },
        ]}
      />
      <div className="filter-row">
        <SelectField label="Tag" value={filters.tag} onChange={(e) => onTag(e.target.value)}>
          <option value="">All tags</option>
          {tagKnown ? null : <option value={filters.tag}>{filters.tag}</option>}
          {tags.map((t) => (
            <option key={t.id} value={t.name}>
              {t.name}
            </option>
          ))}
        </SelectField>
        {channels.length >= 2 ? (
          <SelectField label="Channel" value={filters.channel} onChange={(e) => onChannel(e.target.value)}>
            <option value="">All channels</option>
            {channelKnown ? null : <option value={filters.channel}>Unknown channel</option>}
            {channels.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </SelectField>
        ) : null}
        <TextField
          label="Search"
          type="search"
          ref={searchRef}
          value={search}
          maxLength={MAX_QUERY}
          onChange={(e) => onSearch(e.target.value)}
        />
      </div>
    </div>
  );
}
