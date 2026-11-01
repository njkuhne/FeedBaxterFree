import type { ReactNode } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { loginUrl } from "../lib/next";
import { useNarrow } from "../lib/useMediaQuery";
import { Rail } from "./Rail";
import { TopBar } from "./TopBar";
import { useAuth } from "./AuthProvider";
import NotFound from "../pages/NotFound";

/** Auth gate: signed-out users go to the login screen, forced-change users to change-password. */
export function RequireAuth() {
  const { user } = useAuth();
  const { pathname, search } = useLocation();
  if (!user) return <Navigate to={loginUrl(pathname, search)} replace />;
  if (user.mustChangePassword) {
    const next = pathname + search;
    return <Navigate to={next === "/" ? "/change-password" : `/change-password?next=${encodeURIComponent(next)}`} replace />;
  }
  return <Shell />;
}

function Shell() {
  const narrow = useNarrow();
  return (
    <>
      <a className="skip-link" href="#main">
        Skip to content
      </a>
      <div className="shell">
        {narrow ? <TopBar /> : <Rail />}
        <main id="main" tabIndex={-1} className="content">
          <Outlet />
        </main>
      </div>
    </>
  );
}

/** Renders the not-found page for reviewers on admin-only routes. */
export function AdminOnly({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  return user?.role === "admin" ? <>{children}</> : <NotFound />;
}
