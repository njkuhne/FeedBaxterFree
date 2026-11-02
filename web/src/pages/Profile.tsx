import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { InlineAlert } from "../components/InlineAlert";
import { McpConfig } from "../admin/McpConfig";
import { PageTitle } from "../components/PageTitle";
import { SecretOnce } from "../components/SecretOnce";
import { TextField } from "../components/TextField";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import { checkNewPassword } from "../lib/password";
import { dateOnly } from "../lib/time";
import type { Token } from "../lib/types";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

export default function Profile() {
  usePageTitle("Profile");
  const { user } = useAuth();
  return (
    <>
      <PageTitle>Profile</PageTitle>
      <section className="section" aria-labelledby="account-h">
        <h2 id="account-h" className="section-title">
          Account
        </h2>
        <dl className="facts">
          <dt>Email</dt>
          <dd>{user?.email}</dd>
          <dt>Role</dt>
          <dd>{user?.role === "admin" ? "Admin" : "Reviewer"}</dd>
        </dl>
      </section>
      <PasswordSection />
      <TokensSection />
    </>
  );
}

function PasswordSection() {
  const toast = useToast();
  const [current, setCurrent] = useState("");
  const [fresh, setFresh] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [newError, setNewError] = useState<string | null>(null);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const newRef = useRef<HTMLInputElement | null>(null);
  const confirmRef = useRef<HTMLInputElement | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setFormError(null);
    setNewError(null);
    setConfirmError(null);
    const problem = checkNewPassword(fresh, confirm);
    if (problem) {
      if (problem.field === "new") {
        setNewError(problem.message);
        newRef.current?.focus();
      } else {
        setConfirmError(problem.message);
        confirmRef.current?.focus();
      }
      return;
    }
    setBusy(true);
    try {
      await api.changePassword(current, fresh);
      setCurrent("");
      setFresh("");
      setConfirm("");
      toast.show("Password changed. Other sessions were signed out.");
    } catch (err) {
      setFormError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="section" aria-labelledby="password-h">
      <h2 id="password-h" className="section-title">
        Change password
      </h2>
      <form onSubmit={(e) => void submit(e)} className="form form-narrow">
        {formError ? (
          <p role="alert" className="form-error">
            {formError}
          </p>
        ) : null}
        <TextField
          label="Current password"
          type="password"
          autoComplete="current-password"
          required
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
        />
        <TextField
          ref={newRef}
          label="New password"
          type="password"
          autoComplete="new-password"
          hint="10 to 256 characters."
          error={newError}
          required
          value={fresh}
          onChange={(e) => setFresh(e.target.value)}
        />
        <TextField
          ref={confirmRef}
          label="Confirm new password"
          type="password"
          autoComplete="new-password"
          error={confirmError}
          required
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        <div>
          <Button type="submit" variant="primary" loading={busy} loadingLabel="Saving…">
            Change password
          </Button>
        </div>
      </form>
    </section>
  );
}

function TokensSection() {
  const toast = useToast();
  const [tokens, setTokens] = useState<Token[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [name, setName] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<Token | null>(null);
  const nameRef = useRef<HTMLInputElement | null>(null);

  const load = useCallback(() => {
    setLoadError(null);
    api
      .listTokens()
      .then((r) => setTokens(r.items))
      .catch((e: unknown) => setLoadError(errorMessage(e)));
  }, []);
  useEffect(load, [load]);

  const create = async (e: FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (trimmed.length < 1 || trimmed.length > 64) {
      setNameError("Use 1 to 64 characters.");
      nameRef.current?.focus();
      return;
    }
    setNameError(null);
    setCreating(true);
    try {
      const res = await api.createToken(trimmed);
      setSecret(res.secret);
      setName("");
      load();
    } catch (err) {
      setNameError(errorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  return (
    <section className="section" aria-labelledby="tokens-h">
      <h2 id="tokens-h" className="section-title">
        Personal access tokens
      </h2>
      <p className="soft measure">
        Tokens let your own AI client use the MCP server with your role. Keep them secret and never put them in an
        app.
      </p>
      <form onSubmit={(e) => void create(e)} className="inline-form">
        <TextField
          ref={nameRef}
          label="Token name"
          value={name}
          error={nameError}
          maxLength={64}
          onChange={(e) => setName(e.target.value)}
        />
        <Button type="submit" variant="text" loading={creating} loadingLabel="Creating…">
          Create token
        </Button>
      </form>
      {secret !== null ? (
        <>
          <SecretOnce
            title="Copy this token now"
            value={secret}
            note="It is shown only once. Only a hash is stored."
            onDone={() => setSecret(null)}
          />
          <McpConfig secret={secret} />
        </>
      ) : null}
      {loadError ? (
        <InlineAlert onRetry={load}>Could not load tokens. {loadError}</InlineAlert>
      ) : tokens === null ? (
        <p className="muted">Loading…</p>
      ) : tokens.length === 0 ? (
        <p className="soft">No tokens yet.</p>
      ) : (
        <div className="table-scroll" role="region" aria-label="Personal access tokens" tabIndex={0}>
          <table className="table">
            <caption className="visually-hidden">Personal access tokens</caption>
            <thead>
              <tr>
                <th scope="col">Name</th>
                <th scope="col">Prefix</th>
                <th scope="col">Created</th>
                <th scope="col">Last used</th>
                <th scope="col">
                  <span className="visually-hidden">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {tokens.map((t) => (
                <tr key={t.id}>
                  <td>{t.name}</td>
                  <td className="mono">{t.prefix}…</td>
                  <td>{dateOnly(t.createdAt)}</td>
                  <td>{t.lastUsedAt === null ? "Never" : dateOnly(t.lastUsedAt)}</td>
                  <td className="actions">
                    <Button variant="danger" onClick={() => setRevoking(t)} aria-label={`Revoke token ${t.name}`}>
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {revoking ? (
        <ConfirmDialog
          danger
          title={`Revoke token ${revoking.name}?`}
          body="Anything using this token stops working immediately."
          confirmLabel="Revoke token"
          busyLabel="Revoking…"
          onClose={() => setRevoking(null)}
          onConfirm={async () => {
            await api.deleteToken(revoking.id);
            toast.show("Token revoked.");
            load();
          }}
        />
      ) : null}
    </section>
  );
}
