import { fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mockMatchMedia } from "../test-media";
import { RequireAuth } from "./Shell";
import type { Role } from "../lib/types";

let role: Role = "admin";
vi.mock("./AuthProvider", () => ({
  useAuth: () => ({
    user: { id: "us_1", email: "nick@example.com", role, mustChangePassword: false },
    signOut: () => Promise.resolve(),
  }),
}));

function renderShell() {
  return render(
    <MemoryRouter initialEntries={["/"]}>
      <Routes>
        <Route element={<RequireAuth />}>
          <Route path="*" element={<p>Page body</p>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

let restore: () => void = () => undefined;
beforeEach(() => {
  role = "admin";
  restore = mockMatchMedia(true);
});
afterEach(() => restore());

const menuButton = () => screen.getByRole("button", { name: "Menu" });

describe("TopBar", () => {
  it("Menu button opens the panel and moves focus to the first link", () => {
    renderShell();
    expect(menuButton().getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByRole("link", { name: "Inbox" })).toBeNull();
    fireEvent.click(menuButton());
    expect(menuButton().getAttribute("aria-expanded")).toBe("true");
    const inbox = screen.getByRole("link", { name: "Inbox" });
    expect(inbox.getAttribute("aria-current")).toBe("page");
    expect(document.activeElement).toBe(inbox);
    expect(screen.getByText("nick@example.com").getAttribute("title")).toBe("nick@example.com");
    expect(screen.getByText("Admin")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Sign out" })).toBeTruthy();
  });

  it("Escape closes the panel and returns focus to the Menu button", () => {
    renderShell();
    fireEvent.click(menuButton());
    fireEvent.keyDown(document.activeElement as HTMLElement, { key: "Escape" });
    expect(menuButton().getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(menuButton());
  });

  it("a click outside closes the panel", () => {
    renderShell();
    fireEvent.click(menuButton());
    fireEvent.pointerDown(screen.getByText("Page body"));
    expect(menuButton().getAttribute("aria-expanded")).toBe("false");
  });

  it("navigating closes the panel", () => {
    renderShell();
    fireEvent.click(menuButton());
    fireEvent.click(screen.getByRole("link", { name: "Channels and keys" }));
    expect(menuButton().getAttribute("aria-expanded")).toBe("false");
  });

  it("admin links are hidden for reviewers", () => {
    role = "reviewer";
    renderShell();
    fireEvent.click(menuButton());
    expect(screen.getByRole("link", { name: "Inbox" })).toBeTruthy();
    expect(screen.getByRole("link", { name: "Profile" })).toBeTruthy();
    for (const name of ["Channels and keys", "People", "Settings"]) {
      expect(screen.queryByRole("link", { name })).toBeNull();
    }
    expect(screen.getByText("Reviewer")).toBeTruthy();
  });

  it("admin links are shown for admins", () => {
    renderShell();
    fireEvent.click(menuButton());
    const names = navLinks().map((a) => a.textContent);
    expect(names).toEqual(["Inbox", "Channels and keys", "People", "Settings", "Profile"]);
  });

  it("shell renders the rail on desktop and the top bar on narrow", () => {
    const narrow = renderShell();
    expect(narrow.container.querySelector("header.topbar")).not.toBeNull();
    expect(narrow.container.querySelector(".rail")).toBeNull();
    narrow.unmount();
    restore();
    restore = mockMatchMedia(false);
    const wide = renderShell();
    expect(wide.container.querySelector(".rail")).not.toBeNull();
    expect(wide.container.querySelector("header.topbar")).toBeNull();
    expect(screen.queryByRole("button", { name: "Menu" })).toBeNull();
  });
});

function navLinks(): HTMLAnchorElement[] {
  const nav = screen.getByRole("navigation", { name: "Main" });
  return Array.from(nav.querySelectorAll("a"));
}
