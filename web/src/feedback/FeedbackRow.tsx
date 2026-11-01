import { Fragment } from "react";
import { Link } from "react-router-dom";
import { Checkbox } from "../components/Checkbox";
import { StatusMarker, STATUS_LABEL } from "../components/StatusMarker";
import { TagPill } from "../components/TagPill";
import { absoluteTime, relativeTime } from "../lib/time";
import type { Feedback } from "../lib/types";

const PREVIEW_LENGTH = 200;
const MAX_PILLS = 3;

/** First 200 characters with all white space (line breaks included) collapsed to single spaces. */
export function previewOf(message: string): string {
  return message.replace(/\s+/g, " ").trim().slice(0, PREVIEW_LENGTH);
}

interface FeedbackRowProps {
  item: Feedback;
  showChannel: boolean;
  selected: boolean;
  /** The one row whose link and checkbox are in the tab order. */
  current: boolean;
  /** Where the row was opened from, so the detail page knows whether Back returns to the inbox. */
  origin: "inbox" | "user";
  onToggle: () => void;
  onCurrent: () => void;
}

export function FeedbackRow({ item, showChannel, selected, current, origin, onToggle, onCurrent }: FeedbackRowProps) {
  const preview = previewOf(item.message);
  const parts: string[] = [
    STATUS_LABEL[item.status],
    item.userId ?? "No user ID",
    ...(item.platform ? [item.platform] : []),
    ...(item.appVersion ? [item.appVersion] : []),
    ...(showChannel ? [item.channelName ?? "Deleted channel"] : []),
  ];
  const pills = item.tags.slice(0, MAX_PILLS);
  const more = item.tags.length - pills.length;
  const tabIndex = current ? 0 : -1;
  return (
    <li className={selected ? "row row-selected" : "row"} data-id={item.id}>
      <div className="row-check">
        <Checkbox
          label={`Select: ${preview.slice(0, 40)}`}
          hideLabel
          checked={selected}
          tabIndex={tabIndex}
          onChange={onToggle}
        />
      </div>
      <StatusMarker status={item.status} />
      <div className="row-body">
        <Link
          to={`/feedback/${encodeURIComponent(item.id)}`}
          state={{ fromList: origin }}
          className="row-link"
          tabIndex={tabIndex}
          onFocus={onCurrent}
          onClick={onCurrent}
        >
          <span className="row-preview">{preview}</span>
        </Link>
        <p className="row-meta">
          {parts.map((p, i) => (
            <Fragment key={i}>
              {i > 0 ? " · " : null}
              <span className={i === 0 ? "row-status" : undefined}>{p}</span>
            </Fragment>
          ))}
          {pills.length > 0 ? (
            <span className="row-tags">
              {pills.map((t) => (
                <TagPill key={t} name={t} />
              ))}
              {more > 0 ? <span className="row-more">+{more}</span> : null}
            </span>
          ) : null}
        </p>
      </div>
      <time className="row-time" dateTime={item.receivedAt} title={absoluteTime(item.receivedAt)}>
        {relativeTime(item.receivedAt)}
      </time>
    </li>
  );
}
