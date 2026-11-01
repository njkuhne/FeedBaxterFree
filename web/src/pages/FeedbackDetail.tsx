import { useCallback, useEffect, useId, useState, type FormEvent, type ReactNode } from "react";
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
import { TagPill } from "../components/TagPill";
import { TextField } from "../components/TextField";
import { useToast } from "../components/Toast";
import { ApiError, errorMessage } from "../lib/api";
import {
  addTags,
  deleteFeedback,
  getFeedback,
  listChannels,
  listTags,
  removeTags,
  setStatus,
} from "../lib/endpoints";
import { shortcutFor, type KeyLike } from "../lib/shortcuts";
import { absoluteTime, relativeTime } from "../lib/time";
import type { FeedbackDetail as Detail, Json, Status, TagCount } from "../lib/types";
import "../styles/feedback.css";
import { ACTION_LABEL, ACTION_ORDER, markedOneSentence, TAG_ERROR, validTag } from "../feedback/statusCopy";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

type Load = { phase: "loading" } | { phase: "missing" } | { phase: "error"; message: string } | { phase: "ready"; item: Detail };

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

export default function FeedbackDetail() {
  const { id = "" } = useParams();
  const { user } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const toast = useToast();
  const announce = useAnnounce();
  const [load, setLoad] = useState<Load>({ phase: "loading" });
  const [tags, setTags] = useState<TagCount[]>([]);
  const [channelCount, setChannelCount] = useState(0);
  const [busy, setBusy] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [tagText, setTagText] = useState("");
  const [tagError, setTagError] = useState<string | null>(null);
  const listId = useId();

  const fetchItem = useCallback(() => {
    return getFeedback(id)
      .then((item) => setLoad({ phase: "ready", item }))
      .catch((e: unknown) =>
        setLoad(e instanceof ApiError && e.status === 404 ? { phase: "missing" } : { phase: "error", message: errorMessage(e) }),
      );
  }, [id]);

  useEffect(() => {
    setLoad({ phase: "loading" });
    void fetchItem();
    listTags()
      .then((t) => setTags(t.items))
      .catch(() => undefined);
    listChannels()
      .then((c) => setChannelCount(c.items.length))
      .catch(() => undefined);
  }, [fetchItem]);

  usePageTitle(load.phase === "loading" ? null : load.phase === "missing" ? "Feedback not found" : "Feedback");

  const item = load.phase === "ready" ? load.item : null;
  const status = item?.status ?? null;

  const changeStatus = useCallback(
    async (next: Status) => {
      if (busy || item === null || item.status === next) return;
      setBusy(true);
      try {
        await setStatus(item.id, next);
        await fetchItem();
        announce(markedOneSentence(next));
      } catch (e) {
        toast.show(errorMessage(e), "error");
      } finally {
        setBusy(false);
      }
    },
    [busy, item, fetchItem, announce, toast],
  );

  // e / a / i / n work on the page, as in the list.
  useEffect(() => {
    if (status === null) return;
    const onKey = (e: KeyboardEvent) => {
      const open = document.querySelector('[aria-modal="true"]') !== null;
      const action = shortcutFor(keyLike(e), { dialogOpen: open });
      if (action === "reviewed" || action === "actioned" || action === "ignored" || action === "new") {
        void changeStatus(action);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [status, changeStatus]);

  const goBack = () => {
    const from = (location.state as { fromList?: string } | null)?.fromList;
    if (location.key !== "default" && from === "inbox") navigate(-1);
    else navigate("/");
  };

  if (load.phase === "loading") {
    return (
      <>
        <PageTitle>Feedback</PageTitle>
        <p className="muted">Loading…</p>
      </>
    );
  }
  if (load.phase === "missing") {
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
  if (load.phase === "error") {
    return (
      <>
        <PageTitle>Feedback</PageTitle>
        <InlineAlert onRetry={() => void fetchItem()}>Could not load this feedback. {load.message}</InlineAlert>
      </>
    );
  }

  const f = load.item;
  const primary: Status | null = f.status === "new" ? "reviewed" : f.status === "reviewed" ? "actioned" : null;
  const others = ACTION_ORDER.filter((s) => s !== f.status && s !== primary);
  const metadata = f.metadata ? Object.entries(f.metadata) : [];

  const addTag = async (e: FormEvent) => {
    e.preventDefault();
    const tag = validTag(tagText);
    if (tag === null) {
      setTagError(TAG_ERROR);
      return;
    }
    setTagError(null);
    if (f.tags.some((t) => t.toLowerCase() === tag.toLowerCase())) {
      setTagText("");
      return;
    }
    try {
      await addTags([f.id], [tag]);
      const sorted = [...f.tags, tag].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
      setLoad({ phase: "ready", item: { ...f, tags: sorted } });
      setTagText("");
      announce(`Added tag ${tag}.`);
      listTags()
        .then((t) => setTags(t.items))
        .catch(() => undefined);
    } catch (err) {
      toast.show(errorMessage(err), "error");
    }
  };

  const removeTag = async (tag: string) => {
    try {
      await removeTags([f.id], [tag]);
      setLoad({ phase: "ready", item: { ...f, tags: f.tags.filter((t) => t !== tag) } });
      announce(`Removed tag ${tag}.`);
      listTags()
        .then((t) => setTags(t.items))
        .catch(() => undefined);
    } catch (err) {
      toast.show(errorMessage(err), "error");
    }
  };

  return (
    <div className="detail">
      <Button variant="text" className="detail-back" onClick={goBack}>
        <ArrowLeftIcon />
        Inbox
      </Button>
      <PageTitle>Feedback</PageTitle>
      <div className="detail-meta">
        <StatusChip status={f.status} />
        <span aria-hidden="true">·</span>
        {f.userId !== null ? (
          <Link to={`/user/${encodeURIComponent(f.userId)}`}>{f.userId}</Link>
        ) : (
          <span>No user ID</span>
        )}
        {f.platform ? <MetaPart>{f.platform}</MetaPart> : null}
        {f.appVersion ? <MetaPart>{f.appVersion}</MetaPart> : null}
        {channelCount >= 2 ? <MetaPart>{f.channelName ?? "Deleted channel"}</MetaPart> : null}
        <MetaPart>
          <time dateTime={f.receivedAt}>{absoluteTime(f.receivedAt)}</time>{" "}
          <span className="detail-time">({relativeTime(f.receivedAt)})</span>
        </MetaPart>
      </div>
      <EmailWarning userId={f.userId} />

      <p className="message">{f.message}</p>

      <div className="status-actions">
        {primary !== null ? (
          <Button variant="primary" disabled={busy} onClick={() => void changeStatus(primary)}>
            {ACTION_LABEL[primary]}
          </Button>
        ) : null}
        {others.map((s) => (
          <Button key={s} variant="text" disabled={busy} onClick={() => void changeStatus(s)}>
            {ACTION_LABEL[s]}
          </Button>
        ))}
      </div>

      <section className="section" aria-labelledby={`${listId}-tags`}>
        <h2 className="section-title" id={`${listId}-tags`}>
          Tags
        </h2>
        {f.tags.length > 0 ? (
          <div className="tag-list">
            {f.tags.map((t) => (
              <TagPill key={t} name={t} onRemove={() => void removeTag(t)} />
            ))}
          </div>
        ) : null}
        <form className="tag-form" onSubmit={(e) => void addTag(e)} noValidate>
          <TextField
            label="Tag"
            value={tagText}
            list={`${listId}-suggest`}
            maxLength={64}
            error={tagError}
            onChange={(e) => {
              setTagText(e.target.value);
              setTagError(null);
            }}
          />
          <datalist id={`${listId}-suggest`}>
            {tags.map((t) => (
              <option key={t.id} value={t.name} />
            ))}
          </datalist>
          <Button type="submit" variant="text">
            Add tag
          </Button>
        </form>
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
        <ul className="history">
          <li>Received · {absoluteTime(f.receivedAt)}</li>
          {f.history.map((h, i) => (
            <li key={i}>
              {STATUS_LABEL[h.from]} → {STATUS_LABEL[h.to]} · {h.userEmail ?? "a removed user"} · via{" "}
              {h.via === "mcp" ? "MCP" : "UI"} · {absoluteTime(h.at)}
            </li>
          ))}
        </ul>
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
            toast.show("Feedback deleted.", "success");
            navigate("/");
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
