import { useCallback, useEffect, useState, type FormEvent } from "react";
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
          <form className="inline-form" onSubmit={(e) => void create(e)} noValidate>
            <TextField
              label="New channel name"
              value={name}
              error={nameError}
              onChange={(e) => setName(e.target.value)}
              autoComplete="off"
            />
            <Button type="submit" variant="primary" loading={creating} loadingLabel="Creating…">
              Create channel
            </Button>
          </form>
          <div className="panel-list">
            {channels.map((ch) => (
              <ChannelPanel
                key={ch.id}
                channel={ch}
                keys={keys.filter((k) => k.channelId === ch.id)}
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
