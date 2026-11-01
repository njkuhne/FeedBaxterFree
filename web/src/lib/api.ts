export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

const GENERIC = "Something went wrong. Try again in a moment.";
const NETWORK = "Could not reach the server. Check your connection and try again.";

export type AuthProblem = "unauthenticated" | "password_change_required";
type Subscriber = (problem: AuthProblem) => void;

const subscribers = new Set<Subscriber>();

/** Subscribe to 401 and password_change_required answers. Returns the unsubscribe function. */
export function onAuthProblem(fn: Subscriber): () => void {
  subscribers.add(fn);
  return () => {
    subscribers.delete(fn);
  };
}

function isErrorShape(v: unknown): v is { error: string; message: string } {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  return typeof o["error"] === "string" && typeof o["message"] === "string";
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    "X-Requested-With": "fetch",
    Accept: "application/json",
  };
  const init: RequestInit = { method, credentials: "same-origin", headers };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    throw new ApiError(0, "network", NETWORK);
  }
  if (res.status === 204) return undefined as T;
  let data: unknown;
  try {
    data = await res.json();
  } catch {
    data = undefined;
  }
  if (!res.ok) {
    const err = isErrorShape(data)
      ? new ApiError(res.status, data.error, data.message)
      : new ApiError(res.status, "internal", GENERIC);
    if (err.status === 401) subscribers.forEach((fn) => fn("unauthenticated"));
    else if (err.status === 403 && err.code === "password_change_required") {
      subscribers.forEach((fn) => fn("password_change_required"));
    }
    throw err;
  }
  return data as T;
}

/** The message to show for any thrown value. */
export function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : GENERIC;
}
