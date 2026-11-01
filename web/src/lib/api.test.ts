import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, onAuthProblem, request } from "./api";

type FetchCall = [string, RequestInit];

function mockFetch(res: Response | Error) {
  const fn = vi.fn((_path: string, _init: RequestInit) =>
    res instanceof Error ? Promise.reject(res) : Promise.resolve(res),
  );
  vi.stubGlobal("fetch", fn);
  return fn;
}

function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function lastCall(fn: ReturnType<typeof mockFetch>): FetchCall {
  const call = fn.mock.calls.at(-1);
  if (!call) throw new Error("fetch not called");
  return call as FetchCall;
}

beforeEach(() => vi.unstubAllGlobals());
afterEach(() => vi.unstubAllGlobals());

describe("request", () => {
  it("sends X-Requested-With on every method", async () => {
    for (const method of ["GET", "POST", "PATCH", "DELETE"]) {
      const fn = mockFetch(json(200, {}));
      await request(method, "/api/x");
      const [, init] = lastCall(fn);
      const headers = init.headers as Record<string, string>;
      expect(headers["X-Requested-With"]).toBe("fetch");
      expect(init.method).toBe(method);
      expect(init.credentials).toBe("same-origin");
    }
  });

  it("sets Content-Type only when there is a body", async () => {
    const fn = mockFetch(json(200, {}));
    await request("POST", "/api/x", { a: 1 });
    let [, init] = lastCall(fn);
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
    expect(init.body).toBe('{"a":1}');
    await request("GET", "/api/x");
    [, init] = lastCall(fn);
    expect((init.headers as Record<string, string>)["Content-Type"]).toBeUndefined();
    expect(init.body).toBeUndefined();
  });

  it("returns undefined for 204", async () => {
    mockFetch(new Response(null, { status: 204 }));
    expect(await request("DELETE", "/api/x")).toBeUndefined();
  });

  it("turns an error body into an ApiError", async () => {
    mockFetch(json(409, { error: "conflict", message: "Keep at least one channel." }));
    const err = await request("DELETE", "/api/x").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ status: 409, code: "conflict", message: "Keep at least one channel." });
  });

  it("maps a non-JSON error to internal", async () => {
    mockFetch(new Response("<html>bad gateway</html>", { status: 502 }));
    const err = await request("GET", "/api/x").catch((e: unknown) => e);
    expect(err).toMatchObject({
      status: 502,
      code: "internal",
      message: "Something went wrong. Try again in a moment.",
    });
  });

  it("maps a network failure to code network", async () => {
    mockFetch(new TypeError("Failed to fetch"));
    const err = await request("GET", "/api/x").catch((e: unknown) => e);
    expect(err).toMatchObject({
      status: 0,
      code: "network",
      message: "Could not reach the server. Check your connection and try again.",
    });
  });

  it("notifies the subscriber on 401 and password_change_required only", async () => {
    const seen: string[] = [];
    const off = onAuthProblem((p) => seen.push(p));
    mockFetch(json(401, { error: "unauthenticated", message: "Sign in." }));
    await request("GET", "/api/x").catch(() => undefined);
    mockFetch(json(403, { error: "password_change_required", message: "Change it." }));
    await request("GET", "/api/x").catch(() => undefined);
    mockFetch(json(403, { error: "forbidden", message: "Only admins can do that." }));
    await request("GET", "/api/x").catch(() => undefined);
    mockFetch(json(403, { error: "csrf", message: "No." }));
    await request("GET", "/api/x").catch(() => undefined);
    expect(seen).toEqual(["unauthenticated", "password_change_required"]);
    off();
    mockFetch(json(401, { error: "unauthenticated", message: "Sign in." }));
    await request("GET", "/api/x").catch(() => undefined);
    expect(seen).toHaveLength(2);
  });
});
