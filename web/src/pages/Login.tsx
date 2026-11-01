import { useState, type FormEvent } from "react";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { BaxterSmall } from "../components/Baxter";
import { Button } from "../components/Button";
import { PageTitle } from "../components/PageTitle";
import { TextField } from "../components/TextField";
import { errorMessage } from "../lib/api";
import * as api from "../lib/endpoints";
import { safeNext } from "../lib/next";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

export default function Login() {
  usePageTitle("Sign in");
  const { user, setUser } = useAuth();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const next = safeNext(params.get("next"));
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (user && !busy) {
    return user.mustChangePassword ? <Navigate to={changeUrl(next)} replace /> : <Navigate to={next} replace />;
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const res = await api.login(email, password);
      setUser(res.user);
      navigate(res.user.mustChangePassword ? changeUrl(next) : next, { replace: true });
    } catch (err) {
      setError(errorMessage(err));
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
        <PageTitle>Sign in</PageTitle>
        <form onSubmit={(e) => void submit(e)} className="form">
          {error ? (
            <p role="alert" className="form-error">
              {error}
            </p>
          ) : null}
          <TextField
            label="Email"
            type="email"
            autoComplete="username"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
          />
          <TextField
            label="Password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <Button type="submit" variant="primary" loading={busy} loadingLabel="Signing in…">
            Sign in
          </Button>
        </form>
        <p className="footnote">
          Forgot your password? Ask an admin to reset it. Admins can run <code>feedbaxter reset-admin</code> in the
          container.
        </p>
      </div>
    </div>
  );
}

function changeUrl(next: string): string {
  return next === "/" ? "/change-password" : `/change-password?next=${encodeURIComponent(next)}`;
}
