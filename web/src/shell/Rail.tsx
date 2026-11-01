import { Link, useLocation } from "react-router-dom";
import { BaxterSmall } from "../components/Baxter";
import { Button } from "../components/Button";
import { useAuth } from "./AuthProvider";

/** Inbox is current on `/`, feedback detail and the per-user view. */
function isCurrent(to: string, pathname: string): boolean {
  if (to !== "/") return pathname === to;
  return pathname === "/" || pathname.startsWith("/feedback/") || pathname.startsWith("/user/");
}

export function Rail() {
  const { user, signOut } = useAuth();
  const { pathname } = useLocation();
  if (!user) return null;
  const admin = user.role === "admin";
  const items: { to: string; label: string }[] = [{ to: "/", label: "Inbox" }];
  if (admin) {
    items.push({ to: "/channels", label: "Channels and keys" });
    items.push({ to: "/people", label: "People" });
    items.push({ to: "/settings", label: "Settings" });
  }
  return (
    <nav className="rail" aria-label="Main">
      <Link to="/" className="brand">
        <BaxterSmall size={28} decorative />
        <span className="wordmark">FeedBaxter</span>
      </Link>
      <ul className="rail-nav">
        {items.map((i) => (
          <li key={i.to}>
            <Link to={i.to} className="rail-link" aria-current={isCurrent(i.to, pathname) ? "page" : undefined}>
              {i.label}
            </Link>
          </li>
        ))}
      </ul>
      <div className="rail-account">
        <Link to="/profile" className="rail-email" title={user.email}>
          {user.email}
        </Link>
        <span className="rail-role">{admin ? "Admin" : "Reviewer"}</span>
        <Button variant="text" onClick={() => void signOut()}>
          Sign out
        </Button>
      </div>
    </nav>
  );
}
