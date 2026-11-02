import { useCallback, useEffect, useRef, useState, type FormEvent, type KeyboardEvent } from "react";
import { ChannelPanel } from "../admin/ChannelPanel";
import { Button } from "../components/Button";
import { InlineAlert } from "../components/InlineAlert";
import { PageTitle } from "../components/PageTitle";
import { TextField } from "../components/TextField";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import type { Channel, Key } from "../lib/types";
import { usePageTitle } from "../shell/usePageTitle";
import "../styles/admin.css";

export default function Channels() {
  usePageTitle("Channels and keys");
  const toast = useToast();
  const [channels, setChannels] = useState<Channel[] | null>(null);
  const [keys, setKeys] = useState<Key[]>([]);
  const [counts, setCounts] = useState<Record<string, number> | null>(null);
  const [adding, setAdding] = useState(false);
  const newButton = useRef<HTMLButtonElement | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [deletedTick, setDeletedTick] = useState(0);
  // After the dialog closes (app no longer inert), move focus to the page heading.
  useEffect(() => {
    if (deletedTick > 0) document.querySelector<HTMLElement>("main h1")?.focus();
  }, [deletedTick]);

  const load = useCallback(() => {
    setLoadError(null);
    Promise.all([api.listChannels(), api.listKeys()])
      .then(([c, k]) => {
        setChannels(c.items);
        setKeys(k.items);
      })
      .catch((e: unknown) => setLoadError(errorMessage(e)));
  }, []);
  useEffect(load, [load]);
  // Item counts are a nicety: loaded once, and the page works without them.
  useEffect(() => {
    api
      .stats()
      .then((s) => setCounts(Object.fromEntries(s.byChannel.map((c) => [c.channelId, c.count]))))
      .catch(() => undefined);
  }, []);

  const stopAdding = () => {
    setAdding(false);
    setName("");
    setNameError(null);
    setTimeout(() => newButton.current?.focus(), 0);
  };
  const onFormKey = (e: KeyboardEvent<HTMLFormElement>) => {
    if (e.key === "Escape") {
      e.stopPropagation();
      stopAdding();
    }
  };

  const create = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (trimmed.length < 1 || trimmed.length > 64) {
      setNameError("Use 1 to 64 characters.");
      return;
    }
    setNameError(null);
    setCreating(true);
    try {
      const ch = await api.createChannel(trimmed);
      setChannels((cur) => [...(cur ?? []), ch]);
      setName("");
      setAdding(false);
      setTimeout(() => newButton.current?.focus(), 0);
      toast.show(`Channel ${ch.name} created.`);
    } catch (err) {
      setNameError(errorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  return (
    <>
      <PageTitle>Channels and keys</PageTitle>
      {loadError !== null ? (
        <InlineAlert onRetry={load}>Could not load channels. {loadError}</InlineAlert>
      ) : channels === null ? (
        <p className="muted">Loading channels…</p>
      ) : (
        <>
          {adding ? (
            <form className="inline-form" onSubmit={(e) => void create(e)} onKeyDown={onFormKey} noValidate>
              <TextField
                label="New channel name"
                value={name}
                error={nameError}
                onChange={(e) => setName(e.target.value)}
                autoComplete="off"
                autoFocus
              />
              <Button type="submit" variant="text" loading={creating} loadingLabel="Creating…">
                Create
              </Button>
              <Button variant="text" onClick={stopAdding} disabled={creating}>
                Cancel
              </Button>
            </form>
          ) : (
            <Button ref={newButton} variant="text" className="new-channel" onClick={() => setAdding(true)}>
              New channel
            </Button>
          )}
          <div className="panel-list">
            {channels.map((ch) => (
              <ChannelPanel
                key={ch.id}
                channel={ch}
                keys={keys.filter((k) => k.channelId === ch.id)}
                itemCount={counts === null ? null : (counts[ch.id] ?? 0)}
                channelCount={channels.length}
                onRenamed={(updated) => setChannels((cur) => (cur ?? []).map((c) => (c.id === updated.id ? updated : c)))}
                onDeleted={(gone) => {
                  setChannels((cur) => (cur ?? []).filter((c) => c.id !== gone.id));
                  setKeys((cur) => cur.filter((k) => k.channelId !== gone.id));
                  setDeletedTick((t) => t + 1);
                }}
                onKeyCreated={(key) => setKeys((cur) => [key, ...cur])}
                onKeyRevoked={(key) => setKeys((cur) => cur.map((k) => (k.id === key.id ? key : k)))}
              />
            ))}
          </div>
        </>
      )}
    </>
  );
}
