import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { useAnnounce } from "../components/Announcer";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { EmailWarning } from "../components/EmailWarning";
import { ArrowLeftIcon } from "../components/icons";
import { InlineAlert } from "../components/InlineAlert";
import { PageTitle } from "../components/PageTitle";
import { StatusChip } from "../components/StatusChip";
import { STATUS_LABEL } from "../components/StatusMarker";
import { Highlight } from "../components/Highlight";
import { useToast } from "../components/Toast";
import { ApiError, errorMessage } from "../lib/api";
import {
  addTags,
  deleteFeedback,
  getFeedback,
  listChannels,
  listFeedback,
  listTags,
  removeTags,
  setStatus,
  stats as fetchStats,
} from "../lib/endpoints";
import { matchesFilters } from "../lib/filters";
import { shortcutFor, type KeyLike, type ShortcutAction } from "../lib/shortcuts";
import { platformName } from "../lib/format";
import { absoluteTime, relativeTime } from "../lib/time";
import type { Feedback, HistoryEntry, Json, Status, TagCount } from "../lib/types";
import "../styles/feedback.css";
import { appendPage, applyFeedback, getList, removeFeedback, setFocus, subscribe } from "../feedback/listStore";
import { positionLabel, readFrom } from "../feedback/origin";
import { ACTION_LABEL, ACTION_ORDER, markedOneSentence, markedToast, withUndoHint } from "../feedback/statusCopy";
import { TagEditor, type TagEditorHandle } from "../feedback/TagEditor";
import { useUndo } from "../feedback/undo";
import { useKeyQueue } from "../feedback/useKeyQueue";
import { useNow } from "../feedback/useNow";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

/** The item as the page shows it. History is null while it is still loading. */
type View = Feedback & { history: HistoryEntry[] | null };

type Load =
  | { id: string; phase: "loading" }
  | { id: string; phase: "missing" }
  | { id: string; phase: "error"; message: string }
  | { id: string; phase: "ready"; item: View };

function metaValue(v: Json): string {
  return typeof v === "string" ? v : JSON.stringify(v);
}

function keyLike(e: KeyboardEvent): KeyLike {
  const t = e.target instanceof HTMLElement ? e.target : null;
  return {
    key: e.key,
    ctrlKey: e.ctrlKey,
    metaKey: e.metaKey,
    altKey: e.altKey,
    target: t
      ? { tagName: t.tagName, isContentEditable: t.isContentEditable, type: t instanceof HTMLInputElement ? t.type : undefined }
      : null,
  };
}

/** The item without its history, as the list and Undo hold it. */
function plain(v: View): Feedback {
  const { history, ...rest } = v;
  void history;
  return rest;
}

const detailPath = (id: string) => `/feedback/${encodeURIComponent(id)}`;

export default function FeedbackDetail() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const toast = useToast();
  const announce = useAnnounce();
  const undo = useUndo();
  const now = useNow();
  const from = readFrom(location.state);
  const fromKey = from?.key ?? null;
  const [load, setLoad] = useState<Load>({ id, phase: "loading" });
  const [tags, setTags] = useState<TagCount[]>([]);
  const [channelCount, setChannelCount] = useState(0);
  const [confirming, setConfirming] = useState(false);
  const tagEditor = useRef<TagEditorHandle | null>(null);
  const [paging, setPaging] = useState(false);
  const [advancing, setAdvancing] = useState<string | null>(null);
  const [storeTick, setStoreTick] = useState(0);
  const [statTotal, setStatTotal] = useState<{ key: string; n: number } | null>(null);
  const listId = useId();
  const idRef = useRef(id);
  idRef.current = id;

  const fetchItem = useCallback(() => {
    const wanted = id;
    return getFeedback(wanted)
      .then((item) => {
        if (idRef.current === wanted) setLoad({ id: wanted, phase: "ready", item });
      })
      .catch((e: unknown) => {
        if (idRef.current !== wanted) return;
        setLoad(
          e instanceof ApiError && e.status === 404
            ? { id: wanted, phase: "missing" }
            : { id: wanted, phase: "error", message: errorMessage(e) },
        );
      });
  }, [id]);
  const fetchRef = useRef(fetchItem);
  fetchRef.current = fetchItem;

  useEffect(() => {
    void fetchItem();
    listTags()
      .then((t) => setTags(t.items))
      .catch(() => undefined);
    listChannels()
      .then((c) => setChannelCount(c.items.length))
      .catch(() => undefined);
  }, [fetchItem]);

  // Changes made anywhere reach this page through the store: re-render for the list position,
  // and keep the shown item in step (Undo, tags).
  useEffect(
    () =>
      subscribe((e) => {
        setStoreTick((t) => t + 1);
        if (e.type !== "apply") return;
        const mine = e.items.find((i) => i.id === idRef.current);
        if (mine === undefined) return;
        setLoad((cur) => (cur.phase === "ready" && cur.id === mine.id ? { ...cur, item: { ...cur.item, ...mine } } : cur));
        if (e.reason === "undo") void fetchRef.current();
      }),
    [],
  );

  // After an auto-advance, keys wait until the new item is the one in the URL.
  useEffect(() => {
    if (advancing !== null && advancing === id) setAdvancing(null);
  }, [advancing, id]);

  const cur = load.id === id ? load : null;
  const list = from ? getList(from.key) : undefined;
  const index = list ? list.items.findIndex((i) => i.id === id) : -1;
  const seed = index >= 0 && list ? (list.items[index] ?? null) : null;
  const view: View | null = cur?.phase === "ready" ? cur.item : seed ? { ...seed, history: null } : null;
  const missing = cur?.phase === "missing";

  usePageTitle(missing ? "Feedback not found" : view !== null || cur?.phase === "error" ? "Feedback" : null);

  // The total behind "4 of 17": the loaded rows when the list is complete, otherwise the counts.
  const partial = list !== undefined && list.nextCursor !== null;
  useEffect(() => {
    if (!partial || fromKey === null) return;
    const filters = list?.filters;
    if (filters === undefined) return;
    let live = true;
    fetchStats(filters)
      .then((s) => {
        if (live) setStatTotal({ key: fromKey, n: filters.status === "all" ? s.total : (s.byStatus[filters.status] ?? 0) });
      })
      .catch(() => undefined);
    return () => {
      live = false;
    };
    // The list object changes on every store event, which is exactly when to ask again.
  }, [partial, fromKey, storeTick]);

  const go = (target: string) => navigate(detailPath(target), { replace: true, state: { from } });

  const goBack = (focusId: string | null = id) => {
    if (from !== null && focusId !== null) setFocus(from.key, focusId);
    if (from !== null && location.key !== "default") navigate(-1);
    else navigate(from?.path ?? "/");
  };

  const goPrevious = () => {
    const prev = list?.items[index - 1];
    if (prev) go(prev.id);
  };

  const goNext = async () => {
    if (from === null || list === undefined || index < 0) return;
    const next = list.items[index + 1];
    if (next) {
      go(next.id);
      return;
    }
    if (list.nextCursor === null) return;
    setPaging(true);
    try {
      appendPage(from.key, await listFeedback(from.filters, list.nextCursor));
      const more = getList(from.key)?.items[index + 1];
      if (more) go(more.id);
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setPaging(false);
    }
  };

  const changeStatus = async (next: Status) => {
    if (view === null || view.status === next) return;
    if (!queue.begin()) return;
    const snapshot = plain(view);
    const prevId = list && index > 0 ? (list.items[index - 1]?.id ?? null) : null;
    const run = async (): Promise<boolean> => {
      try {
        const updated = await setStatus(snapshot.id, next);
        applyFeedback([updated], "change");
        announce(withUndoHint(markedOneSentence(next)));
        const message = markedToast(1, next);
        const left = from !== null && index >= 0 && !matchesFilters(updated, from.filters);
        if (from === null || !left) {
          undo.offer({ message, before: [snapshot] });
          await fetchItem();
          return true;
        }
        // The item left the list it was opened from: move on to the one that followed it.
        let after = getList(from.key);
        let target = after?.items[index];
        if (target === undefined && after !== undefined && after.nextCursor !== null) {
          try {
            appendPage(from.key, await listFeedback(from.filters, after.nextCursor));
            after = getList(from.key);
            target = after?.items[index];
          } catch {
            target = undefined;
          }
        }
        if (target === undefined) {
          undo.offer({ message, before: [snapshot] });
          goBack(prevId);
          return true;
        }
        undo.offer({
          message,
          before: [snapshot],
          revisit: { landedOn: detailPath(target.id), path: detailPath(snapshot.id), state: { from } },
        });
        setAdvancing(target.id);
        go(target.id);
        return true;
      } catch (e) {
        toast.show(errorMessage(e), "error");
        return false;
      } finally {
        queue.end();
      }
    };
    await undo.track(run());
  };

  const runAction = (action: ShortcutAction) => {
    switch (action) {
      case "reviewed":
      case "actioned":
      case "ignored":
      case "new":
        void changeStatus(action);
        return;
      case "next":
        void goNext();
        return;
      case "prev":
        goPrevious();
        return;
      case "back":
      case "clearSelection":
        goBack();
        return;
      case "tag":
        tagEditor.current?.open();
        return;
      default:
        return;
    }
  };
  const runRef = useRef(runAction);
  runRef.current = runAction;
  // Keys pressed while a request runs wait and are applied afterwards, against the refreshed item.
  const queue = useKeyQueue<ShortcutAction>((a) => runRef.current(a), advancing !== null && advancing !== id);

  const hasView = view !== null;
  const queueRef = useRef(queue);
  queueRef.current = queue;
  useEffect(() => {
    if (!hasView) return;
    const onKey = (e: KeyboardEvent) => {
      const open = document.querySelector('[aria-modal="true"]') !== null;
      const action = shortcutFor(keyLike(e), { dialogOpen: open });
      if (action === null) return;
      // On this page only the letters step through the list; the arrows keep scrolling.
      if ((action === "next" && e.key !== "j") || (action === "prev" && e.key !== "k")) return;
      if (
        action !== "reviewed" &&
        action !== "actioned" &&
        action !== "ignored" &&
        action !== "new" &&
        action !== "next" &&
        action !== "prev" &&
        action !== "back" &&
        action !== "clearSelection" &&
        action !== "tag"
      ) {
        return;
      }
      // The editor's input takes focus; the key itself must not be typed into it.
      if (action === "tag") e.preventDefault();
      if (queueRef.current.offer(action)) return;
      runRef.current(action);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [hasView]);

  const backLabel = from?.label ?? "Inbox";
  const backName = from === null || from.kind === "inbox" ? "Back to Inbox" : `Back to feedback from ${from.label}`;

  if (missing) {
    return (
      <>
        <PageTitle>Feedback not found</PageTitle>
        <p className="soft">It may have been deleted.</p>
        <Button variant="text" onClick={() => navigate("/")}>
          Back to the inbox
        </Button>
      </>
    );
  }
  if (view === null) {
    if (cur?.phase === "error") {
      return (
        <>
          <PageTitle>Feedback</PageTitle>
          <InlineAlert onRetry={() => void fetchItem()}>Could not load this feedback. {cur.message}</InlineAlert>
        </>
      );
    }
    return (
      <>
        <PageTitle>Feedback</PageTitle>
        <p className="muted">Loading…</p>
      </>
    );
  }

  const f = view;
  const primary: Status | null = f.status === "new" ? "reviewed" : f.status === "reviewed" ? "actioned" : null;
  const metadata = f.metadata ? Object.entries(f.metadata) : [];
  const showNav = from !== null && list !== undefined && index >= 0;
  const hasPrevious = index > 0;
  const hasNext = showNav && (index + 1 < list.items.length || list.nextCursor !== null);
  const total = showNav ? (list.nextCursor === null ? list.items.length : statTotal?.key === fromKey ? statTotal.n : null) : null;

  const addTag = async (tag: string): Promise<boolean> => {
    try {
      await addTags([f.id], [tag]);
      const sorted = [...f.tags, tag].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
      applyFeedback([{ ...plain(f), tags: sorted }], "tags");
      announce(`Added tag ${tag}.`);
      listTags()
        .then((t) => setTags(t.items))
        .catch(() => undefined);
      return true;
    } catch (err) {
      toast.show(errorMessage(err), "error");
      return false;
    }
  };

  const removeTag = async (tag: string) => {
    const snapshot = plain(f);
    try {
      await removeTags([f.id], [tag]);
      applyFeedback([{ ...snapshot, tags: snapshot.tags.filter((t) => t !== tag) }], "tags");
      announce(withUndoHint(`Removed tag ${tag}.`));
      undo.offer({ kind: "tag-remove", message: `Removed tag ${tag}`, tag, before: [snapshot] });
      listTags()
        .then((t) => setTags(t.items))
        .catch(() => undefined);
    } catch (err) {
      toast.show(errorMessage(err), "error");
    }
  };

  return (
    <div className="detail">
      <div className="detail-nav">
        <Button variant="text" className="detail-back" aria-label={backName} onClick={() => goBack()}>
          <ArrowLeftIcon />
          {backLabel}
        </Button>
        {showNav ? (
          <div className="detail-pager">
            <Button variant="text" disabled={!hasPrevious || queue.busy} onClick={goPrevious}>
              Previous
            </Button>
            <span className="detail-position">
              {index + 1}
              {total !== null ? ` of ${total}` : ""} {positionLabel(from)}
            </span>
            <Button
              variant="text"
              disabled={!hasNext || queue.busy}
              loading={paging}
              loadingLabel="Loading…"
              onClick={() => void goNext()}
            >
              Next
            </Button>
          </div>
        ) : null}
      </div>
      <PageTitle hidden>{f.userId !== null ? `Feedback from ${f.userId}` : "Feedback with no user ID"}</PageTitle>
      <div className="detail-meta">
        <StatusChip status={f.status} />
        <span aria-hidden="true">·</span>
        {f.userId !== null ? (
          <Link to={`/user/${encodeURIComponent(f.userId)}`}>{f.userId}</Link>
        ) : (
          <span>No user ID</span>
        )}
        {f.platform ? <MetaPart>{platformName(f.platform)}</MetaPart> : null}
        {f.appVersion ? <MetaPart>{f.appVersion}</MetaPart> : null}
        {channelCount >= 2 ? <MetaPart>{f.channelName ?? "Deleted channel"}</MetaPart> : null}
        <MetaPart>
          <time dateTime={f.receivedAt}>{absoluteTime(f.receivedAt)}</time>{" "}
          <span className="detail-time">({relativeTime(f.receivedAt, new Date(now))})</span>
        </MetaPart>
      </div>
      <EmailWarning userId={f.userId} />

      <p className="message">
        <Highlight text={f.message} query={from?.filters.q ?? ""} />
      </p>

      <div className="status-actions">
        {ACTION_ORDER.map((s) => (
          <Button
            key={s}
            variant={s === primary ? "primary" : "text"}
            disabled={queue.busy || f.status === s}
            onClick={() => void changeStatus(s)}
          >
            {ACTION_LABEL[s]}
          </Button>
        ))}
      </div>

      <section className="section" aria-labelledby={`${listId}-tags`}>
        <h2 className="visually-hidden" id={`${listId}-tags`}>
          Tags
        </h2>
        <TagEditor ref={tagEditor} tags={f.tags} suggestions={tags} onAdd={addTag} onRemove={(t) => void removeTag(t)} />
      </section>

      <section className="section" aria-labelledby={`${listId}-meta`}>
        <h2 className="section-title" id={`${listId}-meta`}>
          Metadata
        </h2>
        {metadata.length > 0 ? (
          <dl className="facts">
            {metadata.map(([k, v]) => (
              <div key={k}>
                <dt>{k}</dt>
                <dd>{metaValue(v)}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="muted">No metadata.</p>
        )}
      </section>

      <section className="section" aria-labelledby={`${listId}-history`}>
        <h2 className="section-title" id={`${listId}-history`}>
          History
        </h2>
        {f.history === null ? (
          <p className="muted">Loading history…</p>
        ) : (
          <ul className="history">
            <li>Received · {absoluteTime(f.receivedAt)}</li>
            {f.history.map((h, i) => (
              <li key={i}>
                {STATUS_LABEL[h.from]} → {STATUS_LABEL[h.to]} · {h.userEmail ?? "a removed user"} · via{" "}
                {h.via === "mcp" ? "MCP" : "UI"} · {absoluteTime(h.at)}
              </li>
            ))}
          </ul>
        )}
      </section>

      <div className="detail-footer">
        {f.userId !== null ? (
          <Link to={`/user/${encodeURIComponent(f.userId)}`}>More from this user →</Link>
        ) : null}
        {user?.role === "admin" ? (
          <Button variant="danger" onClick={() => setConfirming(true)}>
            Delete feedback
          </Button>
        ) : null}
      </div>

      {confirming ? (
        <ConfirmDialog
          danger
          title="Delete this feedback?"
          body="This permanently deletes the feedback. It still counts toward this month's submission limit."
          confirmLabel="Delete feedback"
          busyLabel="Deleting…"
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            await deleteFeedback(f.id);
            const rows = list?.items ?? [];
            const at = rows.findIndex((i) => i.id === f.id);
            const focusId = rows[at + 1]?.id ?? (at > 0 ? rows[at - 1]?.id : undefined) ?? null;
            removeFeedback([f.id]);
            toast.show("Feedback deleted.", "success");
            goBack(focusId);
          }}
        />
      ) : null}
    </div>
  );
}

function MetaPart({ children }: { children: ReactNode }) {
  return (
    <>
      <span aria-hidden="true">·</span>
      <span>{children}</span>
    </>
  );
}
