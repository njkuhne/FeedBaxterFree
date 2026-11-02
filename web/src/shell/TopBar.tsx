import { useEffect, useId, useRef, useState, type FocusEvent, type KeyboardEvent } from "react";
import { Link, useLocation } from "react-router-dom";
import { BaxterSmall } from "../components/Baxter";
import { Button } from "../components/Button";
import { ChevronDownIcon } from "../components/icons";
import { useAuth } from "./AuthProvider";
import { isCurrent, NavLabel, navItems } from "./Rail";

/** The shell below 880 px: a 56 px bar with the brand and a Menu disclosure holding navigation and the account. */
export function TopBar() {
  const { user, signOut } = useAuth();
  const { pathname } = useLocation();
  const [open, setOpen] = useState(false);
  const headerRef = useRef<HTMLElement | null>(null);
  const buttonRef = useRef<HTMLButtonElement | null>(null);
  const panelRef = useRef<HTMLElement | null>(null);
  const panelId = useId();

  // Navigating closes the menu.
  useEffect(() => setOpen(false), [pathname]);

  // Opening puts focus on the first link.
  useEffect(() => {
    if (open) panelRef.current?.querySelector<HTMLElement>("a")?.focus();
  }, [open]);

  // A pointer press outside the bar closes it without moving focus.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: Event) => {
      if (e.target instanceof Node && headerRef.current?.contains(e.target) !== true) setOpen(false);
    };
    document.addEventListener("pointerdown", onDown);
    return () => document.removeEventListener("pointerdown", onDown);
  }, [open]);

  if (!user) return null;
  const admin = user.role === "admin";

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (open && e.key === "Escape") {
      e.preventDefault();
      setOpen(false);
      buttonRef.current?.focus();
    }
  };
  const onBlur = (e: FocusEvent<HTMLElement>) => {
    const next = e.relatedTarget;
    if (open && next instanceof Node && headerRef.current?.contains(next) !== true) setOpen(false);
  };

  return (
    <header className="topbar" ref={headerRef} onKeyDown={onKeyDown} onBlur={onBlur}>
      <Link to="/" className="brand">
        <BaxterSmall size={28} decorative />
        <span className="wordmark">FeedBaxter</span>
      </Link>
      <Button
        variant="text"
        ref={buttonRef}
        className="topbar-menu"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={() => setOpen((o) => !o)}
      >
        Menu
        <ChevronDownIcon className={open ? "topbar-chevron topbar-chevron-open" : "topbar-chevron"} />
      </Button>
      <nav aria-label="Main" id={panelId} className="topbar-panel" hidden={!open} ref={panelRef}>
        <ul className="topbar-nav">
          {navItems(admin).map((i) => (
            <li key={i.to}>
              <Link to={i.to} className="rail-link" aria-current={isCurrent(i.to, pathname) ? "page" : undefined}>
                <NavLabel to={i.to} label={i.label} />
              </Link>
            </li>
          ))}
          <li>
            <Link to="/profile" className="rail-link" aria-current={pathname === "/profile" ? "page" : undefined}>
              Profile
            </Link>
          </li>
        </ul>
        <div className="topbar-account">
          <span className="topbar-email" title={user.email}>
            {user.email}
          </span>
          <span className="rail-role">{admin ? "Admin" : "Reviewer"}</span>
          <Button variant="text" onClick={() => void signOut()}>
            Sign out
          </Button>
        </div>
      </nav>
    </header>
  );
}
