import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { ApiError, errorMessage, onAuthProblem } from "../lib/api";
import * as api from "../lib/endpoints";
import { loginUrl } from "../lib/next";
import type { User } from "../lib/types";
import { useToast } from "../components/Toast";
import { InlineAlert } from "../components/InlineAlert";
import { clearFeedbackCache } from "../feedback/useFeedbackList";

type Phase = "loading" | "ready" | "error";

interface AuthValue {
  user: User | null;
  setUser: (user: User | null) => void;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthValue | null>(null);

export function useAuth(): AuthValue {
  const v = useContext(AuthContext);
  if (!v) throw new Error("useAuth outside AuthProvider");
  return v;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUserState] = useState<User | null>(null);
  const [phase, setPhase] = useState<Phase>("loading");
  const [loadError, setLoadError] = useState("");
  const userRef = useRef<User | null>(null);
  const navigate = useNavigate();
  const location = useLocation();
  const toast = useToast();
  const where = useRef({ pathname: location.pathname, search: location.search });
  where.current = { pathname: location.pathname, search: location.search };

  const setUser = useCallback((u: User | null) => {
    // Cached inbox lists belong to the previous session; drop them on sign-out or a user switch.
    if (u === null || u.id !== userRef.current?.id) clearFeedbackCache();
    userRef.current = u;
    setUserState(u);
  }, []);

  const load = useCallback(() => {
    setPhase("loading");
    api
      .me()
      .then((u) => {
        setUser(u);
        setPhase("ready");
      })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 401) {
          setUser(null);
          setPhase("ready");
        } else {
          setLoadError(errorMessage(e));
          setPhase("error");
        }
      });
  }, [setUser]);

  useEffect(load, [load]);

  useEffect(
    () =>
      onAuthProblem((problem) => {
        const current = userRef.current;
        if (!current) return;
        if (problem === "unauthenticated") {
          setUser(null);
          toast.show("Your session ended. Sign in again.", "info");
          navigate(loginUrl(where.current.pathname, where.current.search), { replace: true });
        } else {
          setUser({ ...current, mustChangePassword: true });
          navigate("/change-password", { replace: true });
        }
      }),
    [navigate, setUser, toast],
  );

  const signOut = useCallback(async () => {
    try {
      await api.logout();
    } catch {
      // The session may already be gone; leave anyway.
    }
    setUser(null);
    navigate("/login", { replace: true });
  }, [navigate, setUser]);

  const value = useMemo(() => ({ user, setUser, signOut }), [user, setUser, signOut]);

  if (phase === "loading") return <BootLoading />;
  if (phase === "error") {
    return (
      <div className="boot">
        <InlineAlert onRetry={load}>Could not reach FeedBaxter. {loadError}</InlineAlert>
      </div>
    );
  }
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

/** Blank for the first 300 ms, then "Loading…". */
function BootLoading() {
  const [late, setLate] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setLate(true), 300);
    return () => clearTimeout(t);
  }, []);
  return <div className="boot">{late ? <p className="muted">Loading…</p> : null}</div>;
}
