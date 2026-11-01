import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { SelectField } from "../components/SelectField";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import { shortDate } from "../lib/time";
import type { PersonUser, Role } from "../lib/types";

interface PeopleTableProps {
  users: PersonUser[];
  currentUserId: string;
  /** A role was changed on the server. */
  onRoleChanged: (user: PersonUser) => void;
  /** Reset succeeded; the temporary password is shown once by the page. */
  onReset: (user: PersonUser, temporaryPassword: string) => void;
  onRemoved: (user: PersonUser) => void;
}

export function PeopleTable({ users, currentUserId, onRoleChanged, onReset, onRemoved }: PeopleTableProps) {
  const toast = useToast();
  const [pending, setPending] = useState<string | null>(null);
  const [resetting, setResetting] = useState<PersonUser | null>(null);
  const [removing, setRemoving] = useState<PersonUser | null>(null);
  const adminCount = users.filter((u) => u.role === "admin").length;

  const changeRole = async (user: PersonUser, role: Role) => {
    if (role === user.role) return;
    setPending(user.id);
    try {
      const updated = await api.setRole(user.id, role);
      const selfDemoted = user.id === currentUserId && role === "reviewer";
      if (!selfDemoted) toast.show(`${user.email} is now ${role === "admin" ? "an admin" : "a reviewer"}.`);
      onRoleChanged(updated);
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setPending(null);
    }
  };

  return (
    <div className="table-scroll" tabIndex={0} aria-label="People">
      <table className="table">
        <caption className="visually-hidden">People</caption>
        <thead>
          <tr>
            <th scope="col">Email</th>
            <th scope="col">Role</th>
            <th scope="col">Added</th>
            <th scope="col">
              <span className="visually-hidden">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {users.map((u) => {
            const self = u.id === currentUserId;
            const lastAdmin = u.role === "admin" && adminCount === 1;
            return (
              <tr key={u.id}>
                <td className="person-email">
                  <span>
                    {u.email}
                    {self ? " (you)" : ""}
                  </span>
                  {u.mustChangePassword ? <span className="muted person-note">Temporary password</span> : null}
                </td>
                <td>
                  <SelectField
                    label={`Role for ${u.email}`}
                    hideLabel
                    className="role-select"
                    value={u.role}
                    disabled={lastAdmin || pending === u.id}
                    hint={lastAdmin ? "The last admin cannot be removed or made a reviewer." : undefined}
                    onChange={(e) => void changeRole(u, e.target.value === "admin" ? "admin" : "reviewer")}
                  >
                    <option value="reviewer">Reviewer</option>
                    <option value="admin">Admin</option>
                  </SelectField>
                </td>
                <td>{shortDate(u.createdAt)}</td>
                <td className="actions">
                  {self ? (
                    <span className="muted">
                      Change your own password in <Link to="/profile">your profile</Link>.
                    </span>
                  ) : (
                    <>
                      <Button variant="text" onClick={() => setResetting(u)}>
                        Reset password
                      </Button>
                      {lastAdmin ? null : (
                        <Button variant="danger" onClick={() => setRemoving(u)}>
                          Remove
                        </Button>
                      )}
                    </>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      {resetting ? (
        <ConfirmDialog
          title={`Reset the password for ${resetting.email}?`}
          body="Their sessions end and they get a temporary password to change at next sign-in."
          confirmLabel="Reset password"
          busyLabel="Resetting…"
          onConfirm={async () => {
            const r = await api.resetPassword(resetting.id);
            onReset(resetting, r.temporaryPassword);
          }}
          onClose={() => setResetting(null)}
        />
      ) : null}
      {removing ? (
        <ConfirmDialog
          danger
          title={`Remove ${removing.email}?`}
          body="They are signed out and their personal access tokens stop working."
          confirmLabel="Remove person"
          busyLabel="Removing…"
          onConfirm={async () => {
            await api.deleteUser(removing.id);
            toast.show(`${removing.email} removed.`);
            onRemoved(removing);
          }}
          onClose={() => setRemoving(null)}
        />
      ) : null}
    </div>
  );
}
