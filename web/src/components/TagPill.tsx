import { Link } from "react-router-dom";
import { XIcon } from "./icons";

interface TagPillProps {
  name: string;
  onRemove?: () => void;
  /** Makes the pill a link that filters the list by this tag. */
  to?: string;
  /** For the link: follows the row's roving tab index. */
  tabIndex?: number;
}

export function TagPill({ name, onRemove, to, tabIndex }: TagPillProps) {
  if (onRemove !== undefined) {
    return (
      <span className="pill pill-removable">
        {name}
        <button type="button" className="pill-remove" aria-label={`Remove tag ${name}`} onClick={onRemove}>
          <XIcon />
        </button>
      </span>
    );
  }
  if (to !== undefined) {
    return (
      <Link to={to} className="pill pill-link" aria-label={`Filter by tag ${name}`} {...(tabIndex === undefined ? {} : { tabIndex })}>
        {name}
      </Link>
    );
  }
  return <span className="pill">{name}</span>;
}
