import { XIcon } from "./icons";

interface TagPillProps {
  name: string;
  onRemove?: () => void;
}

export function TagPill({ name, onRemove }: TagPillProps) {
  if (onRemove === undefined) return <span className="pill">{name}</span>;
  return (
    <span className="pill pill-removable">
      {name}
      <button type="button" className="pill-remove" aria-label={`Remove tag ${name}`} onClick={onRemove}>
        <XIcon />
      </button>
    </span>
  );
}
