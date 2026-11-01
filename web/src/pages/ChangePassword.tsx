import { useRef, useState, type FormEvent } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "../components/Button";
import { BaxterSmall } from "../components/Baxter";
import { PageTitle } from "../components/PageTitle";
import { useToast } from "../components/Toast";
import { TextField } from "../components/TextField";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import { safeNext } from "../lib/next";
import { checkNewPassword } from "../lib/password";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

export default function ChangePassword() {
  usePageTitle("Choose a new password");
  const { user, setUser, signOut } = useAuth();
  const navigate = useNavigate();
  const toast = useToast();
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  const [current, setCurrent] = useState("");
  const [fresh, setFresh] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [newError, setNewError] = useState<string | null>(null);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const newRef = useRef<HTMLInputElement | null>(null);
  const confirmRef = useRef<HTMLInputElement | null>(null);

  if (!user) return <Navigate to="/login" replace />;
  if (!user.mustChangePassword && !busy) return <Navigate to={next} replace />;

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
      setUser(await api.me());
      toast.show("Password changed.");
      navigate(next, { replace: true });
    } catch (err) {
      setFormError(errorMessage(err));
      setBusy(false);
    }
  };

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="brand brand-static">
          <BaxterSmall size={28} decorative />
          <span className="wordmark">FeedBaxter</span>
        </div>
        <PageTitle>Choose a new password</PageTitle>
        <p className="soft">You signed in with a temporary password. Choose a new one to continue.</p>
        <form onSubmit={(e) => void submit(e)} className="form">
          {formError ? (
            <p role="alert" className="form-error">
              {formError}
            </p>
          ) : null}
          <TextField
            label="Temporary password"
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
          <Button type="submit" variant="primary" loading={busy} loadingLabel="Saving…">
            Save password
          </Button>
          <Button variant="text" onClick={() => void signOut()}>
            Sign out
          </Button>
        </form>
      </div>
    </div>
  );
}
