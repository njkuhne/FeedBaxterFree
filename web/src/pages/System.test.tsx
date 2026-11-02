import { render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { mockApi, type Call } from "../feedback/testHelpers";
import { routes } from "../routes";
import System from "./System";

vi.mock("../shell/AuthProvider", () => ({
  useAuth: () => ({
    user: { id: "us_1", email: "a@example.com", role: "admin", mustChangePassword: false },
    signOut: () => Promise.resolve(),
  }),
}));

const ops = (dlq: number, version = "0.4.0") => ({
  queue: { depth: 0, oldestAgeSeconds: 0, dlq },
  counters: { day: 1, month: 2, dayKey: "d", monthKey: "m" },
  dbSizeBytes: 1024,
  version,
  mode: "all",
  dialect: "sqlite",
});
const serve = (dlq: number, version?: string) =>
  mockApi((c: Call) => (c.path === "/api/ops" ? { body: ops(dlq, version) } : undefined));

beforeEach(() => vi.unstubAllGlobals());

describe("System page", () => {
  it("settings redirects to system", async () => {
    serve(0);
    const settings = routes[0]?.children?.find((r) => "element" in r && r.element !== undefined && "children" in r);
    const inner = settings && "children" in settings ? settings.children : undefined;
    const paths = inner?.map((r) => r.path);
    expect(paths).toContain("/system");
    expect(paths).toContain("/settings");
    render(
      <MemoryRouter initialEntries={["/settings"]}>
        <Routes>
          {inner?.map((r) => (
            <Route key={r.path} path={r.path} element={r.path === "/profile" ? null : r.element} />
          ))}
        </Routes>
      </MemoryRouter>,
    );
    expect(await screen.findByRole("heading", { level: 1, name: "System" })).toBeTruthy();
  });

  it("shows the version in plain words in the footer", async () => {
    serve(0);
    render(
      <MemoryRouter>
        <System />
      </MemoryRouter>,
    );
    expect(await screen.findByText("FeedBaxter 0.4.0 · single process · SQLite")).toBeTruthy();
    expect(screen.queryByText("Version")).toBeNull();
  });

  it("could not be stored shows an alert icon and a line when above zero", async () => {
    serve(3);
    render(
      <MemoryRouter>
        <System />
      </MemoryRouter>,
    );
    const label = await screen.findByText("Could not be stored");
    expect(label.parentElement?.querySelector("dd svg")).not.toBeNull();
    expect(screen.getByText(/3 submissions could not be stored\. They are kept aside/)).toBeTruthy();
  });

  it("shows no icon or note at zero", async () => {
    serve(0);
    render(
      <MemoryRouter>
        <System />
      </MemoryRouter>,
    );
    const label = await screen.findByText("Could not be stored");
    expect(label.parentElement?.querySelector("dd svg")).toBeNull();
    expect(screen.queryByText(/could not be stored\. They/)).toBeNull();
  });
});
