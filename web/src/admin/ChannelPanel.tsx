import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SecretOnce } from "../components/SecretOnce";
import { TextField } from "../components/TextField";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import { plural } from "../lib/format";
import { relativeTime, shortDate } from "../lib/time";
import type { Channel, Key } from "../lib/types";
import { KeyTable } from "./KeyTable";

interface ChannelPanelProps {
  channel: Channel;
  keys: Key[];
  /** Feedback items on this channel, or null while unknown. */
  itemCount?: number | null;
  /** How many channels exist; the last one cannot be deleted. */
  channelCount: number;
  onRenamed: (channel: Channel) => void;
  onDeleted: (channel: Channel) => void;
  onKeyCreated: (key: Key) => void;
  onKeyRevoked: (key: Key) => void;
}

export function deleteBody(activeKeys: number): string {
  if (activeKeys === 0) return "It has no active keys. Feedback already received is kept.";
  const noun = activeKeys === 1 ? "1 active key is" : `${activeKeys} active keys are`;
  return `Its ${noun} revoked immediately, so apps using them will start failing. Feedback already received is kept.`;
}

/** "312 items · last received 2 h ago", or "No feedback yet". Null when nothing is known (field absent, no count). */
export function receivedLine(count: number | null, lastReceivedAt: string | null | undefined): string | null {
  if (count === 0 || lastReceivedAt === null) return "No feedback yet";
  if (lastReceivedAt === undefined) return count === null ? null : plural(count, "item", "items");
  const items = count === null ? "" : `${plural(count, "item", "items")} · `;
  return `${items}last received ${relativeTime(lastReceivedAt)}`;
}

export function ChannelPanel({
  channel,
  keys,
  itemCount = null,
  channelCount,
  onRenamed,
  onDeleted,
  onKeyCreated,
  onKeyRevoked,
}: ChannelPanelProps) {
  const toast = useToast();
  const headingId = useId();
  const heading = useRef<HTMLHeadingElement | null>(null);
  const renameButton = useRef<HTMLButtonElement | null>(null);
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(channel.name);
  const [renameError, setRenameError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [revoking, setRevoking] = useState<Key | null>(null);
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const revoked = useRef(false);
  const [headingTick, setHeadingTick] = useState(0);
  // Runs after the dialog has closed and the app is no longer inert.
  useEffect(() => {
    if (headingTick > 0) heading.current?.focus();
  }, [headingTick]);
  const line = receivedLine(itemCount, channel.lastReceivedAt);
  const activeKeys = keys.filter((k) => k.revokedAt === null).length;

  const startRename = () => {
    setName(channel.name);
    setRenameError(null);
    setRenaming(true);
  };
  const stopRename = () => {
    setRenaming(false);
    setRenameError(null);
    setTimeout(() => renameButton.current?.focus(), 0);
  };
  const saveRename = async () => {
    const trimmed = name.trim();
    if (trimmed.length < 1 || trimmed.length > 64) {
      setRenameError("Use 1 to 64 characters.");
      return;
    }
    setSaving(true);
    setRenameError(null);
    try {
      const updated = await api.renameChannel(channel.id, trimmed);
      onRenamed(updated);
      toast.show("Channel renamed.");
      setRenaming(false);
      setTimeout(() => renameButton.current?.focus(), 0);
    } catch (e) {
      setRenameError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };
  const onRenameKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      void saveRename();
    } else if (e.key === "Escape") {
      e.stopPropagation();
      stopRename();
    }
  };

  const createKey = async () => {
    setCreating(true);
    try {
      const r = await api.createKey(channel.id);
      onKeyCreated(r.key);
      setSecret(r.secret);
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setCreating(false);
    }
  };

  return (
    <section className="channel-panel" aria-labelledby={headingId}>
      <div className="channel-head">
        {renaming ? (
          <div className="channel-rename" onKeyDown={onRenameKey}>
            <TextField
              label="Channel name"
              value={name}
              error={renameError}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <Button variant="text" loading={saving} loadingLabel="Saving…" onClick={() => void saveRename()}>
              Save
            </Button>
            <Button variant="text" onClick={stopRename} disabled={saving}>
              Cancel
            </Button>
          </div>
        ) : null}
        <>
            <div className={renaming ? "visually-hidden" : "channel-title"}>
              <h2 id={headingId} ref={heading} tabIndex={-1} className="channel-name">
                {channel.name}
              </h2>
              {line !== null ? <span className="muted">{line}</span> : null}
              <span className="muted">created {shortDate(channel.createdAt)}</span>
            </div>
            <div className="channel-actions" hidden={renaming}>
              <Button ref={renameButton} variant="text" onClick={startRename}>
                Rename
              </Button>
            </div>
        </>
      </div>

      <div className="keys-head">
        <h3 className="keys-title">Keys</h3>
        <Button variant="text" loading={creating} loadingLabel="Creating…" onClick={() => void createKey()}>
          Create key
        </Button>
      </div>
      {secret !== null ? (
        <SecretOnce
          title="Copy this key now"
          value={secret}
          note="It is shown only once. Only a hash is stored."
          onDone={() => setSecret(null)}
        />
      ) : null}
      <KeyTable channelName={channel.name} keys={keys} onRevoke={setRevoking} />

      <div className="channel-danger">
        {channelCount > 1 ? (
          <Button variant="danger" onClick={() => setDeleting(true)}>
            Delete channel
          </Button>
        ) : (
          <span className="muted">The only channel cannot be deleted.</span>
        )}
      </div>

      {deleting ? (
        <ConfirmDialog
          danger
          title={`Delete channel ${channel.name}?`}
          body={deleteBody(activeKeys)}
          confirmLabel="Delete channel"
          busyLabel="Deleting…"
          onConfirm={async () => {
            await api.deleteChannel(channel.id);
            toast.show(`Channel ${channel.name} deleted.`);
            onDeleted(channel);
          }}
          onClose={() => setDeleting(false)}
        />
      ) : null}
      {revoking ? (
        <ConfirmDialog
          danger
          title={`Revoke key ${revoking.prefix}…?`}
          body="Apps using this key will start failing immediately. This cannot be undone."
          confirmLabel="Revoke key"
          busyLabel="Revoking…"
          onConfirm={async () => {
            const updated = await api.revokeKey(revoking.id);
            onKeyRevoked(updated);
            toast.show("Key revoked.");
            revoked.current = true;
          }}
          onClose={() => {
            setRevoking(null);
            if (revoked.current) {
              revoked.current = false;
              setHeadingTick((t) => t + 1);
            }
          }}
        />
      ) : null}
    </section>
  );
}
