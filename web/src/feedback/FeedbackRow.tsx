import { Fragment, type MouseEvent as ReactMouseEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { Checkbox } from "../components/Checkbox";
import { StatusMarker, STATUS_LABEL } from "../components/StatusMarker";
import { Highlight } from "../components/Highlight";
import { TagPill } from "../components/TagPill";
import { filtersToQuery } from "../lib/filters";
import { platformName } from "../lib/format";
import { absoluteTime, relativeTime } from "../lib/time";
import type { Feedback } from "../lib/types";
import type { FromList } from "./origin";

const PREVIEW_LENGTH = 300;
const MAX_PILLS = 3;
const WINDOW_AFTER = 120;
const WINDOW_BEFORE = 40;

/** The whole message, trimmed, runs of spaces and tabs collapsed, three or more line breaks collapsed to two. */
function normalised(message: string): string {
  return message
    .replace(/\r\n?/g, "\n")
    .trim()
    .replace(/[ \t]+/g, " ")
    .replace(/ ?\n ?/g, "\n")
    .replace(/\n{3,}/g, "\n\n");
}

/** Trimmed, runs of spaces and tabs collapsed, three or more line breaks collapsed to two, cut at 300 characters. */
export function previewOf(message: string): string {
  return normalised(message).slice(0, PREVIEW_LENGTH);
}

/**
 * The text a row shows. With a query whose first match starts after character 120, the preview
 * starts at a word boundary about 40 characters before the match, prefixed with "…", so the match
 * is visible inside the two-line clamp.
 */
export function previewFor(message: string, query: string): string {
  const full = normalised(message);
  const q = query.trim().toLowerCase();
  const lower = full.toLowerCase();
  if (q === "" || lower.length !== full.length) return full.slice(0, PREVIEW_LENGTH);
  const hit = lower.indexOf(q);
  if (hit <= WINDOW_AFTER) return full.slice(0, PREVIEW_LENGTH);
  let start = hit - WINDOW_BEFORE;
  while (start < hit && !/\s/.test(full.charAt(start))) start += 1;
  while (start < hit && /\s/.test(full.charAt(start))) start += 1;
  return `…${full.slice(start, start + PREVIEW_LENGTH)}`;
}

interface FeedbackRowProps {
  item: Feedback;
  /** The status word in the meta line: only when the list is not already a single status. */
  showStatus: boolean;
  showChannel: boolean;
  showUser: boolean;
  selected: boolean;
  /** The one row whose link and checkbox are in the tab order. */
  current: boolean;
  /** Where the row is listed, so the detail page can go back and step through the same list. */
  from: FromList;
  /** Clock for the relative time (ms since epoch). */
  now?: number;
  /** `range` is true for a Shift-click. */
  onToggle: (range: boolean) => void;
  onCurrent: () => void;
}

export function FeedbackRow({
  item,
  showStatus,
  showChannel,
  showUser,
  selected,
  current,
  from,
  now,
  onToggle,
  onCurrent,
}: FeedbackRowProps) {
  const navigate = useNavigate();
  const query = from.filters.q;
  const preview = previewOf(item.message);
  const shown = previewFor(item.message, query);
  // A tag pill filters by that tag: the inbox keeps status, channel and search; the per-user page goes to the inbox.
  const tagHref = (tag: string) =>
    from.kind === "user"
      ? `/?${new URLSearchParams({ status: "all", tag }).toString()}`
      : `/?${filtersToQuery({ ...from.filters, tag }).toString()}`;
  const path = `/feedback/${encodeURIComponent(item.id)}`;
  const parts: { text: string; className?: string }[] = [
    ...(showStatus ? [{ text: STATUS_LABEL[item.status], className: "row-status" }] : []),
    ...(showUser ? [{ text: item.userId ?? "No user ID" }] : []),
    ...(item.platform ? [{ text: platformName(item.platform) }] : []),
    ...(item.appVersion ? [{ text: item.appVersion }] : []),
    ...(showChannel ? [{ text: item.channelName ?? "Deleted channel" }] : []),
  ];
  const pills = item.tags.slice(0, MAX_PILLS);
  const more = item.tags.length - pills.length;
  const tabIndex = current ? 0 : -1;
  const classes = ["row", selected ? "row-selected" : "", current ? "row-current" : ""].filter(Boolean).join(" ");

  // The whole row opens the item, except clicks on controls and while text is being selected.
  const open = (e: ReactMouseEvent<HTMLLIElement>) => {
    if (e.defaultPrevented) return;
    if (e.target instanceof Element && e.target.closest("a, button, input, label, select") !== null) return;
    if ((window.getSelection()?.toString() ?? "") !== "") return;
    onCurrent();
    void navigate(path, { state: { from } });
  };

  return (
    <li className={classes} data-id={item.id} onClick={open}>
      <div className="row-check">
        <Checkbox
          label={`Select: ${preview.replace(/\s+/g, " ").slice(0, 40)}`}
          hideLabel
          checked={selected}
          tabIndex={tabIndex}
          onChange={(e) => onToggle(e.nativeEvent instanceof MouseEvent && e.nativeEvent.shiftKey)}
        />
      </div>
      <StatusMarker status={item.status} {...(showStatus ? {} : { label: STATUS_LABEL[item.status] })} />
      <div className="row-body">
        <Link to={path} state={{ from }} className="row-link" tabIndex={tabIndex} onFocus={onCurrent} onClick={onCurrent}>
          <span className="row-preview">
            <Highlight text={shown} query={query} />
          </span>
        </Link>
        <p className="row-meta">
          {parts.map((p, i) => (
            <Fragment key={i}>
              {i > 0 ? " · " : null}
              <span className={p.className}>{p.text}</span>
            </Fragment>
          ))}
          {pills.length > 0 ? (
            <span className={parts.length > 0 ? "row-tags row-tags-after" : "row-tags"}>
              {pills.map((t) => (
                <TagPill key={t} name={t} to={tagHref(t)} tabIndex={tabIndex} />
              ))}
              {more > 0 ? <span className="row-more">+{more}</span> : null}
            </span>
          ) : null}
        </p>
      </div>
      <time className="row-time" dateTime={item.receivedAt} title={absoluteTime(item.receivedAt)}>
        {relativeTime(item.receivedAt, now === undefined ? new Date() : new Date(now))}
      </time>
    </li>
  );
}
