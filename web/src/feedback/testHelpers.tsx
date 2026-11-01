import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { vi } from "vitest";
import { AnnouncerProvider } from "../components/Announcer";
import { ToastProvider } from "../components/Toast";
import type { Feedback } from "../lib/types";
import { UndoProvider } from "./undo";

export function fb(over: Partial<Feedback> & { id: string }): Feedback {
  return {
    channelId: "ch_1",
    channelName: "Default",
    message: `Message ${over.id}`,
    userId: "cust-1",
    appVersion: "2.3.0",
    platform: "ios",
    metadata: null,
    status: "new",
    receivedAt: "2026-03-01T09:30:00.000Z",
    updatedAt: "2026-03-01T09:30:00.000Z",
    updatedBy: null,
    tags: [],
    ...over,
  };
}

export interface Call {
  method: string;
  path: string;
  body: unknown;
}

export interface Answer {
  status?: number;
  body?: unknown;
  /** The response is held back until this promise resolves, to test requests in flight. */
  after?: Promise<unknown>;
}

export type Handler = (call: Call) => Answer | undefined;

/** Stubs fetch. The handler answers by method and path; anything it does not answer is 404. */
export function mockApi(handler: Handler): { calls: Call[] } {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((path: string, init?: RequestInit) => {
      const body: unknown = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
      const call = { method: init?.method ?? "GET", path, body };
      calls.push(call);
      const answer = handler(call) ?? { status: 404, body: { error: "not_found", message: "Not found." } };
      const status = answer.status ?? 200;
      const response =
        status === 204
          ? new Response(null, { status })
          : new Response(JSON.stringify(answer.body ?? {}), { status, headers: { "Content-Type": "application/json" } });
      return answer.after ? answer.after.then(() => response) : Promise.resolve(response);
    }),
  );
  return { calls };
}

export function renderPage(ui: ReactElement, opts: { at?: string; route?: string } = {}) {
  return render(
    <ToastProvider>
      <AnnouncerProvider>
        <MemoryRouter initialEntries={[opts.at ?? "/"]}>
          <UndoProvider>
            <Routes>
              <Route path={opts.route ?? "*"} element={ui} />
            </Routes>
          </UndoProvider>
        </MemoryRouter>
      </AnnouncerProvider>
    </ToastProvider>,
  );
}

export interface HistoryEntry {
  /** May carry a query string: "/?status=reviewed". */
  pathname: string;
  state?: unknown;
}

/** Renders real routes with a history stack; the last entry is the current page. */
export function renderRoutes(entries: HistoryEntry[], routes: { path: string; element: ReactElement }[]) {
  const initial = entries.map((e) => {
    const q = e.pathname.indexOf("?");
    return {
      pathname: q < 0 ? e.pathname : e.pathname.slice(0, q),
      search: q < 0 ? "" : e.pathname.slice(q),
      state: e.state,
    };
  });
  return render(
    <ToastProvider>
      <AnnouncerProvider>
        <MemoryRouter initialEntries={initial} initialIndex={initial.length - 1}>
          <UndoProvider>
            <Routes>
              {routes.map((r) => (
                <Route key={r.path} path={r.path} element={r.element} />
              ))}
            </Routes>
          </UndoProvider>
        </MemoryRouter>
      </AnnouncerProvider>
    </ToastProvider>,
  );
}
