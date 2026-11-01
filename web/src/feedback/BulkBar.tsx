import { useState, type FormEvent } from "react";
import { Button } from "../components/Button";
import { TextField } from "../components/TextField";
import type { Status } from "../lib/types";
import { ACTION_LABEL, ACTION_ORDER, TAG_ERROR, validTag } from "./statusCopy";

interface BulkBarProps {
  count: number;
  /** Number of items in the running request, 0 when idle. */
  busy: number;
  onStatus: (status: Status) => void;
  /** Resolves true when the tag was added. */
  onAddTag: (tag: string) => Promise<boolean>;
  onClear: () => void;
}

export function BulkBar({ count, busy, onStatus, onAddTag, onClear }: BulkBarProps) {
  const [tag, setTag] = useState("");
  const [error, setError] = useState<string | null>(null);
  const working = busy > 0;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const valid = validTag(tag);
    if (valid === null) {
      setError(TAG_ERROR);
      return;
    }
    setError(null);
    if (await onAddTag(valid)) setTag("");
  };

  return (
    <div className="bulk" role="toolbar" aria-label="Bulk actions">
      <span className="bulk-count">{working ? `Updating ${busy}…` : `${count} selected`}</span>
      {ACTION_ORDER.map((s) => (
        <Button key={s} variant="text" disabled={working} onClick={() => onStatus(s)}>
          {ACTION_LABEL[s]}
        </Button>
      ))}
      <form className="bulk-tag" onSubmit={(e) => void submit(e)} noValidate>
        <TextField
          label="Tag"
          value={tag}
          maxLength={64}
          disabled={working}
          error={error}
          onChange={(e) => {
            setTag(e.target.value);
            setError(null);
          }}
        />
        <Button type="submit" variant="text" disabled={working}>
          Add tag
        </Button>
      </form>
      <Button variant="text" className="bulk-clear" disabled={working} onClick={onClear}>
        Clear selection
      </Button>
    </div>
  );
}
