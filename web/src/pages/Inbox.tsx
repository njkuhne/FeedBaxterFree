import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { usePolling } from "../admin/usePolling";
import { useAnnounce } from "../components/Announcer";
import { Button } from "../components/Button";
import { EmptyState } from "../components/EmptyState";
import { InlineAlert } from "../components/InlineAlert";
import { PageTitle } from "../components/PageTitle";
import { Segmented } from "../components/Segmented";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { listChannels, listFeedback, listTags, stats as fetchStats } from "../lib/endpoints";
import { defaultFilters, filtersFromQuery, filtersToQuery } from "../lib/filters";
import { useNarrow } from "../lib/useMediaQuery";
import type { Channel, Filters, Stats, Status, TagCount } from "../lib/types";
import { emptyCopy, reviewedWaiting } from "../feedback/emptyCopy";
import { FeedbackList } from "../feedback/FeedbackList";
import type { FilterChip } from "../feedback/ListHeader";
import { NewPill } from "../feedback/NewPill";
import { Toolbar } from "../feedback/Toolbar";
import { subscribe } from "../feedback/listStore";
import { usePageTitle } from "../shell/usePageTitle";
import Onboarding from "./Onboarding";

interface Boot {
  stats: Stats;
  /** Whether any feedback exists at all, from the unfiltered first load. Filtered counts never reset it. */
  hasFeedback: boolean;
  channels: Channel[];
  tags: TagCount[];
  /** Items per channel from the unfiltered first load, for the Channel menu. */
  channelCounts: Record<string, number>;
}

const SEARCH_DEBOUNCE_MS = 300;
const STATS_POLL_MS = 30_000;

/** A single token that could be a user ID: no white space, 1 to 128 characters. */
const isSingleToken = (q: string) => q.length >= 1 && q.length <= 128 && !/\s/.test(q);

/**
 * For a search that found nothing: if the text is a user ID that has feedback (the exact userId
 * filter, any status), offer the per-user page. One request per query; a late answer is ignored.
 */
function UserIdOffer({ q }: { q: string }) {
  const [found, setFound] = useState<string | null>(null);
  useEffect(() => {
    setFound(null);
    if (!isSingleToken(q)) return;
    let live = true;
    listFeedback({ ...defaultFilters, status: "all", userId: q }, null, 1)
      .then((page) => {
        if (live && page.items.length > 0) setFound(q);
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [q]);
  if (found !== q) return null;
  return (
    <Link className="btn btn-text" to={`/user/${encodeURIComponent(q)}`}>
      {`Show feedback from ${q}`}
    </Link>
  );
}

export default function Inbox() {
  const [params, setParams] = useSearchParams();
  const filters = useMemo(() => filtersFromQuery(params), [params]);
  const toast = useToast();
  const announce = useAnnounce();
  const narrow = useNarrow();
  const [boot, setBoot] = useState<Boot | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState(filters.q);
  const searchRef = useRef<HTMLInputElement | null>(null);
  // The total that matched the current list load, and the latest total seen by polling.
  const [totals, setTotals] = useState<{ baseline: number | null; latest: number }>({ baseline: null, latest: 0 });
  const [reload, setReload] = useState({ key: 0, focusFirst: false });
  const pillRef = useRef<HTMLButtonElement | null>(null);
  // The baseline is only set once the inbox shows a loaded list; polls during onboarding must not set it.
  const listReady = useRef(false);
  const statsKey = useRef<string | null>(null);
  // The tab that was active when a search began, and whether the user picked a tab since.
  const beforeSearch = useRef<{ status: Status | "all"; picked: boolean } | null>(null);

  const load = useCallback(() => {
    setError(null);
    Promise.all([fetchStats(), listChannels(), listTags()])
      .then(([stats, channels, tags]) =>
        setBoot({
          stats,
          hasFeedback: stats.total > 0,
          channels: channels.items,
          tags: tags.items,
          channelCounts: Object.fromEntries(stats.byChannel.map((c) => [c.channelId, c.count])),
        }),
      )
      .catch((e: unknown) => setError(errorMessage(e)));
  }, []);
  useEffect(load, [load]);

  const filtersRef = useRef(filters);
  filtersRef.current = filters;
  /** Fetches the counts for the current filters. "own" is the user's change: their deletions never look like arrivals. */
  const fetchCounts = useCallback((kind: "poll" | "own") => {
    const asked = filtersRef.current;
    return fetchStats(asked)
      .then((stats) => {
        setBoot((b) => (b ? { ...b, stats } : b));
        // An answer for filters the user has since left says nothing about the current list.
        if (!listReady.current) return;
        if (filtersRef.current.tag !== asked.tag || filtersRef.current.channel !== asked.channel || filtersRef.current.q !== asked.q) return;
        setTotals((cur) => {
          const baseline = cur.baseline === null ? stats.total : kind === "own" ? Math.min(cur.baseline, stats.total) : cur.baseline;
          return { baseline, latest: stats.total };
        });
      })
      .catch(() => undefined);
  }, []);
  const refreshStats = useCallback(() => void fetchCounts("own"), [fetchCounts]);
  // The inbox is always fresh to within 30 s while the tab is visible; rows still only change when the user asks.
  usePolling(() => fetchCounts("poll"), STATS_POLL_MS);
  const refreshTags = useCallback(() => {
    listTags()
      .then((t) => setBoot((b) => (b ? { ...b, tags: t.items } : b)))
      .catch(() => undefined);
  }, []);

  // Any change to feedback, from the list or the detail page, refreshes the counts (and tags).
  useEffect(
    () =>
      subscribe((e) => {
        refreshStats();
        if (e.type === "apply" && e.reason === "tags") refreshTags();
      }),
    [refreshStats, refreshTags],
  );

  // Counts follow the tag, channel and search filters (the status tabs stay unfiltered by status).
  const ready = boot !== null && boot.hasFeedback;
  listReady.current = ready;
  useEffect(() => {
    if (!ready) return;
    // Changing the tag, channel or search starts a new baseline; becoming ready keeps the one Onboarding handed over.
    const k = `${filters.tag}|${filters.channel}|${filters.q}`;
    if (statsKey.current !== null && statsKey.current !== k) setTotals({ baseline: null, latest: 0 });
    statsKey.current = k;
    void fetchCounts("poll");
  }, [ready, filters.tag, filters.channel, filters.q, fetchCounts]);

  // Writes the filters to the URL. A search that begins (empty to non-empty) switches to the All tab and
  // remembers the tab it left; when the search ends the tab returns, unless the user picked one meanwhile.
  const commit = (next: Filters, replace = false) => {
    const cur = filtersRef.current;
    const wasEmpty = cur.q.trim() === "";
    const isEmpty = next.q.trim() === "";
    let out = next;
    if (wasEmpty && !isEmpty) {
      beforeSearch.current = { status: cur.status, picked: false };
      out = { ...next, status: "all" };
    } else if (!wasEmpty && isEmpty) {
      const b = beforeSearch.current;
      beforeSearch.current = null;
      if (b !== null && !b.picked) out = { ...next, status: b.status };
    }
    setParams(filtersToQuery(out), { replace });
  };
  const commitRef = useRef(commit);
  commitRef.current = commit;

  // The URL is the source of truth for the search text; typing writes it back after a pause.
  useEffect(() => setSearch(filters.q), [filters.q]);
  useEffect(() => {
    const t = setTimeout(() => {
      const cur = filtersRef.current;
      if (cur.q.trim() === search.trim()) return;
      commitRef.current({ ...cur, q: search }, true);
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(t);
  }, [search]);

  const empty = boot !== null && !boot.hasFeedback;
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
          listReady.current = true;
          setTotals({ baseline: stats.total, latest: stats.total });
          setBoot({ ...boot, stats, hasFeedback: true });
          toast.show("First feedback received.", "success");
        }}
      />
    );
  }

  const push = (next: Filters) => commit(next);
  const hasOtherFilters = filters.tag !== "" || filters.channel !== "" || filters.q.trim() !== "";
  const count = filters.status === "all" ? boot.stats.total : (boot.stats.byStatus[filters.status] ?? 0);
  const clearSearch = () => {
    setSearch("");
    commit({ ...filters, q: "" }, true);
  };
  const clearAll = () => {
    setSearch("");
    commit({ ...filters, tag: "", channel: "", q: "" });
  };
  const channelName = boot.channels.find((c) => c.id === filters.channel)?.name ?? "unknown";
  const chips: FilterChip[] = [
    ...(filters.tag !== "" ? [{ key: "tag", label: `tag ${filters.tag}`, onRemove: () => push({ ...filters, tag: "" }) }] : []),
    ...(filters.channel !== ""
      ? [{ key: "channel", label: `channel ${channelName}`, onRemove: () => push({ ...filters, channel: "" }) }]
      : []),
    ...(filters.q.trim() !== "" ? [{ key: "q", label: `\u201c${filters.q.trim()}\u201d`, onRemove: clearSearch }] : []),
  ];
  const statusCount = (st: Status) => boot.stats.byStatus[st] ?? 0;
  const arrived = totals.baseline === null ? 0 : Math.max(0, totals.latest - totals.baseline);
  const showPill = arrived > 0 && (filters.status === "new" || filters.status === "all");
  const showNew = () => {
    const focusFirst = pillRef.current !== null && document.activeElement === pillRef.current;
    announce(`Showing ${arrived} new ${arrived === 1 ? "item" : "items"}.`);
    setTotals((cur) => ({ baseline: cur.latest, latest: cur.latest }));
    setReload((r) => ({ key: r.key + 1, focusFirst }));
  };
  const reviewed = statusCount("reviewed");

  return (
    <>
      <div className="page-head">
        <PageTitle>Inbox</PageTitle>
      </div>
      <Segmented<Status | "all">
        name="status"
        label="Status"
        value={filters.status}
        onChange={(status) => {
          if (beforeSearch.current !== null) beforeSearch.current.picked = true;
          push({ ...filters, status });
        }}
        options={[
          { value: "new", label: "New", count: statusCount("new") },
          { value: "reviewed", label: "Reviewed", count: statusCount("reviewed") },
          { value: "actioned", label: "Actioned", count: statusCount("actioned") },
          { value: "ignored", label: "Ignored", count: statusCount("ignored") },
          { value: "all", label: "All", count: boot.stats.total },
        ]}
      />
      <Toolbar
        filters={filters}
        search={search}
        searchRef={searchRef}
        tags={boot.tags}
        channels={boot.channels}
        channelCounts={boot.channelCounts}
        count={count}
        onSearch={setSearch}
        onClearSearch={clearSearch}
        onTag={(tag) => push({ ...filters, tag })}
        onChannel={(channel) => push({ ...filters, channel })}
        narrow={narrow}
      />
      {showPill ? <NewPill ref={pillRef} count={arrived} onShow={showNew} /> : null}
      <FeedbackList
        reload={reload}
        filters={filters}
        showChannel={boot.channels.length >= 2}
        origin="inbox"
        originLabel="Inbox"
        searchRef={searchRef}
        count={count}
        chips={chips}
        onClearAll={clearAll}
        tags={boot.tags}
        renderEmpty={(ref) => {
          const copy = emptyCopy(filters, channelName);
          if (!hasOtherFilters && filters.status === "new") {
            if (reviewed > 0) {
              return (
                <EmptyState
                  headingRef={ref}
                  heading={copy.heading}
                  body={reviewedWaiting(reviewed)}
                  action={
                    <Button variant="text" onClick={() => push({ ...filters, status: "reviewed" })}>
                      {`Show ${reviewed} reviewed`}
                    </Button>
                  }
                />
              );
            }
            return <EmptyState headingRef={ref} heading={copy.heading} body={copy.body} />;
          }
          const q = filters.q.trim();
          return (
            <EmptyState
              mark={false}
              headingRef={ref}
              heading={copy.heading}
              body={copy.body}
              action={
                hasOtherFilters ? (
                  <>
                    {q !== "" ? <UserIdOffer q={q} /> : null}
                    <Button variant="text" onClick={clearAll}>
                      Clear filters
                    </Button>
                  </>
                ) : undefined
              }
            />
          );
        }}
      />
    </>
  );
}
