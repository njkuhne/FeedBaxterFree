import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { EmailWarning } from "../components/EmailWarning";
import { PageTitle } from "../components/PageTitle";
import { Segmented } from "../components/Segmented";
import { deleteByUser, listChannels, listFeedback, listTags, stats as fetchStats } from "../lib/endpoints";
import { defaultFilters } from "../lib/filters";
import { plural } from "../lib/format";
import { STATUSES, type Feedback, type Filters, type Stats, type Status, type TagCount } from "../lib/types";
import { ExportMenu } from "../feedback/ExportMenu";
import { FeedbackList } from "../feedback/FeedbackList";
import { getList, listKey, subscribe } from "../feedback/listStore";
import { userSummary } from "../feedback/userSummary";
import { useNow } from "../feedback/useNow";
import { clearFeedbackCache } from "../feedback/useFeedbackList";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

type Tab = Status | "all";

function tabFromQuery(raw: string | null): Tab {
  return raw !== null && (STATUSES as readonly string[]).includes(raw) ? (raw as Status) : "all";
}

interface AllRows {
  items: Feedback[];
  complete: boolean;
}

/**
 * The rows of the user's All list, for the summary line, whichever tab is showing. Read from the list
 * store and re-read on its events; fetched once (first page) when the store does not have it by `ready`.
 */
function useAllRows(allFilters: Filters, ready: boolean): AllRows | null {
  const key = listKey(allFilters);
  const [rows, setRows] = useState<{ key: string; value: AllRows } | null>(null);
  const fetched = useRef<string | null>(null);

  useEffect(() => {
    const read = () => {
      const cached = getList(key);
      if (cached === undefined) return false;
      setRows({ key, value: { items: cached.items, complete: cached.nextCursor === null } });
      return true;
    };
    let live = true;
    const off = subscribe(() => {
      read();
    });
    if (ready && !read() && fetched.current !== key) {
      // Can double up with the list below on the All tab: the list stores its page silently, so a late store read misses it.
      fetched.current = key;
      listFeedback(allFilters)
        .then((page) => {
          if (live) setRows({ key, value: { items: page.items, complete: page.nextCursor === null } });
        })
        .catch(() => undefined);
    }
    return () => {
      live = false;
      off();
    };
    // Per user and readiness, not per render.
  }, [key, ready]);

  return rows !== null && rows.key === key ? rows.value : null;
}

export default function PerUser() {
  const { userId = "" } = useParams();
  const { user } = useAuth();
  const [params, setParams] = useSearchParams();
  const tab = tabFromQuery(params.get("status"));
  const now = useNow();
  usePageTitle(`Feedback from ${userId}`);
  const allFilters = useMemo<Filters>(() => ({ ...defaultFilters, status: "all", userId }), [userId]);
  const filters = useMemo<Filters>(() => ({ ...allFilters, status: tab }), [allFilters, tab]);
  const [channelCount, setChannelCount] = useState(0);
  const [tags, setTags] = useState<TagCount[]>([]);
  const [counts, setCounts] = useState<Stats | null>(null);
  const [settled, setSettled] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [deleted, setDeleted] = useState<number | null>(null);
  const doneRef = useRef<HTMLParagraphElement | null>(null);
  const rows = useAllRows(allFilters, settled);

  useEffect(() => {
    listChannels()
      .then((c) => setChannelCount(c.items.length))
      .catch(() => undefined);
    listTags()
      .then((t) => setTags(t.items))
      .catch(() => undefined);
  }, []);

  // Counts for the tabs, refreshed on any change to feedback (like the inbox).
  const refreshCounts = useCallback(() => {
    fetchStats(allFilters)
      .then((s) => {
        setCounts(s);
        setSettled(true);
      })
      .catch(() => setSettled(true));
  }, [allFilters]);
  useEffect(() => {
    setCounts(null);
    setSettled(false);
    refreshCounts();
    return subscribe(refreshCounts);
  }, [refreshCounts]);

  useEffect(() => {
    if (deleted !== null) doneRef.current?.focus();
  }, [deleted]);

  const statusCount = (st: Status) => counts?.byStatus[st] ?? 0;
  const total = counts?.total ?? null;
  const tabCount = counts === null ? null : tab === "all" ? counts.total : statusCount(tab);
  const summary = rows === null ? null : userSummary(rows.items, total, rows.complete, new Date(now));

  const confirmTitle =
    total === null
      ? `Delete all feedback from ${userId}?`
      : total === 1
        ? `Delete the 1 feedback item from ${userId}?`
        : `Delete all ${total} feedback items from ${userId}?`;

  if (deleted !== null) {
    return (
      <>
        <div className="page-head">
          <PageTitle>Feedback from {userId}</PageTitle>
          <ExportMenu filters={filters} count={null} />
        </div>
        <p role="status" className="user-done" ref={doneRef} tabIndex={-1}>
          Deleted {plural(deleted, "feedback item", "feedback items")} from {userId}.
        </p>
        <Link className="btn btn-text" to="/">
          Back to the inbox
        </Link>
      </>
    );
  }

  const option = (value: Tab, label: string, n: number | undefined) => ({
    value,
    label,
    ...(counts !== null && n !== undefined ? { count: n } : {}),
  });

  return (
    <>
      <div className="page-head">
        <PageTitle>Feedback from {userId}</PageTitle>
        <ExportMenu filters={filters} count={tabCount} />
      </div>
      {summary !== null ? <p className="muted user-summary">{summary}</p> : null}
      <EmailWarning userId={userId} />
      <Segmented<Tab>
        name="user-status"
        label="Status"
        value={tab}
        onChange={(next) => {
          const out = new URLSearchParams(params);
          if (next === "all") out.delete("status");
          else out.set("status", next);
          setParams(out);
        }}
        options={[
          option("all", "All", counts?.total),
          option("new", "New", statusCount("new")),
          option("reviewed", "Reviewed", statusCount("reviewed")),
          option("actioned", "Actioned", statusCount("actioned")),
          option("ignored", "Ignored", statusCount("ignored")),
        ]}
      />
      <FeedbackList
        key={userId}
        filters={filters}
        showChannel={channelCount >= 2}
        tags={tags}
        origin="user"
        originLabel={userId}
        count={tabCount}
        renderEmpty={(ref) => (
          <p className="empty-plain" ref={ref} tabIndex={-1}>
            {tab === "all" ? "No feedback from this user." : `No ${tab} feedback from this user.`}
          </p>
        )}
      />
      {user?.role === "admin" ? (
        <section className="section user-actions" aria-labelledby="user-removal">
          <h2 className="section-title" id="user-removal">
            Data removal
          </h2>
          <p className="soft">
            Use this when your customer asks you to remove this person&apos;s data. It permanently deletes every item from{" "}
            {userId}.
          </p>
          <Button variant="danger" onClick={() => setConfirming(true)}>
            Delete all feedback for this user
          </Button>
        </section>
      ) : null}
      {confirming ? (
        <ConfirmDialog
          danger
          title={confirmTitle}
          body="This permanently deletes every feedback item from this user. Deleted feedback still counts toward this month's submission limit."
          confirmLabel="Delete all feedback"
          busyLabel="Deleting…"
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            const r = await deleteByUser(userId);
            clearFeedbackCache();
            setDeleted(r.deleted);
          }}
        />
      ) : null}
    </>
  );
}
