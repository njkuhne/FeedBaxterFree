import { Link, useLocation } from "react-router-dom";
import { BaxterSmall } from "../components/Baxter";
import { Button } from "../components/Button";
import { useShortcuts } from "../feedback/shortcuts";
import { VisuallyHidden } from "../components/VisuallyHidden";
import { useAuth } from "./AuthProvider";
import { useNewCount } from "./NewCount";

/** Inbox is current on `/`, feedback detail and the per-user view. */
export function isCurrent(to: string, pathname: string): boolean {
  if (to !== "/") return pathname === to;
  return pathname === "/" || pathname.startsWith("/feedback/") || pathname.startsWith("/user/");
}

/** "Inbox" with the New count beside it: visible "Inbox 17", spoken "Inbox, 17 new". */
export function NavLabel({ to, label }: { to: string; label: string }) {
  const count = useNewCount();
  if (to !== "/" || count === null || count <= 0) return <>{label}</>;
  return (
    <>
      {label}
      <span className="nav-count" aria-hidden="true">
        {count}
      </span>
      <VisuallyHidden>{`, ${count} new`}</VisuallyHidden>
    </>
  );
}

/** The main navigation, shared by the rail and the compact top bar. */
export function navItems(admin: boolean): { to: string; label: string }[] {
  const items: { to: string; label: string }[] = [{ to: "/", label: "Inbox" }];
  if (admin) {
    items.push({ to: "/channels", label: "Channels and keys" });
    items.push({ to: "/people", label: "People" });
    items.push({ to: "/system", label: "System" });
  }
  return items;
}

export function Rail() {
  const { user, signOut } = useAuth();
  const { pathname } = useLocation();
  const shortcuts = useShortcuts();
  if (!user) return null;
  const admin = user.role === "admin";
  const items = navItems(admin);
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
              <NavLabel to={i.to} label={i.label} />
            </Link>
          </li>
        ))}
      </ul>
      <div className="rail-account">
        <span className="rail-email" title={user.email}>
          {user.email}
        </span>
        <span className="rail-role">{admin ? "Admin" : "Reviewer"}</span>
        <Link to="/profile" className="btn btn-text" aria-current={pathname === "/profile" ? "page" : undefined}>
          Profile
        </Link>
        <Button variant="text" onClick={shortcuts.open}>
          Keyboard shortcuts
        </Button>
        <Button variant="text" onClick={() => void signOut()}>
          Sign out
        </Button>
      </div>
    </nav>
  );
}
