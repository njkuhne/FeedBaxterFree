import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { vi } from "vitest";
import { AnnouncerProvider } from "../components/Announcer";
import { ToastProvider } from "../components/Toast";
import type { Feedback } from "../lib/types";

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

export type Handler = (call: Call) => { status?: number; body?: unknown } | undefined;

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
      if (status === 204) return Promise.resolve(new Response(null, { status }));
      return Promise.resolve(
        new Response(JSON.stringify(answer.body ?? {}), { status, headers: { "Content-Type": "application/json" } }),
      );
    }),
  );
  return { calls };
}

export function renderPage(ui: ReactElement, opts: { at?: string; route?: string } = {}) {
  return render(
    <ToastProvider>
      <AnnouncerProvider>
        <MemoryRouter initialEntries={[opts.at ?? "/"]}>
          <Routes>
            <Route path={opts.route ?? "*"} element={ui} />
          </Routes>
        </MemoryRouter>
      </AnnouncerProvider>
    </ToastProvider>,
  );
}
