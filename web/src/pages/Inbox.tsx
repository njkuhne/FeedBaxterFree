import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { Button } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { InlineAlert } from "../components/InlineAlert";
import { PageTitle } from "../components/PageTitle";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { listChannels, listTags, stats as fetchStats } from "../lib/endpoints";
import { filtersFromQuery, filtersToQuery, toApiParams } from "../lib/filters";
import type { Channel, Stats, Status, TagCount } from "../lib/types";
import { ExportLinks } from "../feedback/ExportLinks";
import { FeedbackList } from "../feedback/FeedbackList";
import { FilterBar } from "../feedback/FilterBar";
import { usePageTitle } from "../shell/usePageTitle";
import Onboarding from "./Onboarding";

interface Boot {
  stats: Stats;
  channels: Channel[];
  tags: TagCount[];
}

const SEARCH_DEBOUNCE_MS = 300;

export default function Inbox() {
  const [params, setParams] = useSearchParams();
  const filters = useMemo(() => filtersFromQuery(params), [params]);
  const toast = useToast();
  const [boot, setBoot] = useState<Boot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState(filters.q);
  const searchRef = useRef<HTMLInputElement | null>(null);

  const load = useCallback(() => {
    setError(null);
    Promise.all([fetchStats(), listChannels(), listTags()])
      .then(([stats, channels, tags]) => setBoot({ stats, channels: channels.items, tags: tags.items }))
      .catch((e: unknown) => setError(errorMessage(e)));
  }, []);
  useEffect(load, [load]);

  const refreshStats = useCallback(() => {
    fetchStats()
      .then((stats) => setBoot((b) => (b ? { ...b, stats } : b)))
      .catch(() => undefined);
  }, []);
  const refreshTags = useCallback(() => {
    listTags()
      .then((t) => setBoot((b) => (b ? { ...b, tags: t.items } : b)))
      .catch(() => undefined);
  }, []);

  // The URL is the source of truth for the search text; typing writes it back after a pause.
  useEffect(() => setSearch(filters.q), [filters.q]);
  useEffect(() => {
    const t = setTimeout(() => {
      setParams((prev) => {
        const cur = filtersFromQuery(prev);
        if (cur.q.trim() === search.trim()) return prev;
        return filtersToQuery({ ...cur, q: search });
      }, { replace: true });
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search, setParams]);

  const empty = boot !== null && boot.stats.total === 0;
  usePageTitle(boot === null ? null : empty ? "Send your first feedback" : "Inbox");

  if (boot === null) {
    return (
      <>
        <PageTitle>Inbox</PageTitle>
        {error ? <InlineAlert onRetry={load}>Could not load the inbox. {error}</InlineAlert> : <p className="muted">Loading feedback…</p>}
      </>
    );
  }

  if (empty) {
    return (
      <Onboarding
        onReceived={(stats) => {
          setBoot({ ...boot, stats });
          toast.show("First feedback received.", "success");
        }}
      />
    );
  }

  const push = (next: typeof filters) => setParams(filtersToQuery(next));
  const inboxZero = filters.status === "new" && filters.tag === "" && filters.channel === "" && filters.q.trim() === "";

  return (
    <>
      <div className="page-head">
        <PageTitle>Inbox</PageTitle>
        <ExportLinks filters={filters} />
      </div>
      <FilterBar
        filters={filters}
        stats={boot.stats}
        tags={boot.tags}
        channels={boot.channels}
        search={search}
        searchRef={searchRef}
        onStatus={(status: Status | "all") => push({ ...filters, status })}
        onTag={(tag) => push({ ...filters, tag })}
        onChannel={(channel) => push({ ...filters, channel })}
        onSearch={setSearch}
      />
      <FeedbackList
        key={toApiParams(filters).toString()}
        filters={filters}
        showChannel={boot.channels.length >= 2}
        origin="inbox"
        searchRef={searchRef}
        onChanged={refreshStats}
        onTagsChanged={refreshTags}
        renderEmpty={(ref) =>
          inboxZero ? (
            <EmptyState headingRef={ref} heading="No new feedback" body="New feedback appears here as it arrives." />
          ) : (
            <EmptyState
              headingRef={ref}
              heading="Nothing matches these filters"
              body="Try another status or tag, or clear the search."
              action={
                <Button variant="text" onClick={() => setParams(new URLSearchParams())}>
                  Clear filters
                </Button>
              }
            />
          )
        }
      />
    </>
  );
}
