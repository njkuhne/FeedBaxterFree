import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { PeopleTable } from "../admin/PeopleTable";
import { Button } from "../components/Button";
import { InlineAlert } from "../components/InlineAlert";
import { PageTitle } from "../components/PageTitle";
import { SecretOnce } from "../components/SecretOnce";
import { SelectField } from "../components/SelectField";
import { TextField } from "../components/TextField";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import type { PersonUser, Role } from "../lib/types";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";
import "../styles/admin.css";

const PASSWORD_NOTE = "Share it with them directly. They must change it when they first sign in. It is shown only once.";

export default function People() {
  usePageTitle("People");
  const toast = useToast();
  const navigate = useNavigate();
  const { user: me, setUser } = useAuth();
  const [users, setUsers] = useState<PersonUser[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<Role>("reviewer");
  const [emailError, setEmailError] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [secret, setSecret] = useState<{ email: string; value: string } | null>(null);

  const load = useCallback(() => {
    setLoadError(null);
    api
      .listUsers()
      .then((r) => setUsers(r.items))
      .catch((e: unknown) => setLoadError(errorMessage(e)));
  }, []);
  useEffect(load, [load]);

  const add = async (e: FormEvent) => {
    e.preventDefault();
    setEmailError(null);
    setAdding(true);
    try {
      const r = await api.createUser(email.trim(), role);
      setUsers((cur) => [...(cur ?? []), r.user]);
      setSecret({ email: r.user.email, value: r.temporaryPassword });
      setEmail("");
      setRole("reviewer");
    } catch (err) {
      setEmailError(errorMessage(err));
    } finally {
      setAdding(false);
    }
  };

  const roleChanged = (updated: PersonUser) => {
    setUsers((cur) => (cur ?? []).map((u) => (u.id === updated.id ? updated : u)));
    if (me && updated.id === me.id && updated.role === "reviewer") {
      api
        .me()
        .then((fresh) => setUser(fresh))
        .catch(() => setUser({ ...me, role: "reviewer" }))
        .finally(() => {
          toast.show("You are now a reviewer.");
          navigate("/");
        });
    }
  };

  return (
    <>
      <PageTitle>People</PageTitle>
      {loadError !== null ? (
        <InlineAlert onRetry={load}>Could not load people. {loadError}</InlineAlert>
      ) : users === null || me === null ? (
        <p className="muted">Loading people…</p>
      ) : (
        <>
          <form className="inline-form" onSubmit={(e) => void add(e)}>
            <TextField
              label="Email"
              type="email"
              autoComplete="off"
              value={email}
              error={emailError}
              onChange={(e) => setEmail(e.target.value)}
            />
            <SelectField
              label="Role"
              className="role-field"
              value={role}
              onChange={(e) => setRole(e.target.value === "admin" ? "admin" : "reviewer")}
            >
              <option value="reviewer">Reviewer</option>
              <option value="admin">Admin</option>
            </SelectField>
            <Button type="submit" variant="primary" loading={adding} loadingLabel="Adding…">
              Add person
            </Button>
          </form>
          {secret !== null ? (
            <SecretOnce
              title={`Temporary password for ${secret.email}`}
              value={secret.value}
              note={PASSWORD_NOTE}
              onDone={() => setSecret(null)}
            />
          ) : null}
          <PeopleTable
            users={users}
            currentUserId={me.id}
            onRoleChanged={roleChanged}
            onReset={(u, pw) => {
              setUsers((cur) => (cur ?? []).map((x) => (x.id === u.id ? { ...x, mustChangePassword: true } : x)));
              setSecret({ email: u.email, value: pw });
            }}
            onRemoved={(u) => setUsers((cur) => (cur ?? []).filter((x) => x.id !== u.id))}
          />
        </>
      )}
    </>
  );
}
