import { request } from "./api";
import { toApiParams } from "./filters";
import type {
  BulkResult,
  Channel,
  Feedback,
  FeedbackDetail,
  FeedbackPage,
  Filters,
  Items,
  Key,
  Ops,
  PersonUser,
  Role,
  Stats,
  Status,
  TagCount,
  Token,
  User,
} from "./types";

const seg = encodeURIComponent;

// Auth and me
export const login = (email: string, password: string) =>
  request<{ user: User }>("POST", "/api/auth/login", { email, password });
export const logout = () => request<void>("POST", "/api/auth/logout");
export const changePassword = (currentPassword: string, newPassword: string) =>
  request<void>("POST", "/api/auth/password", { currentPassword, newPassword });
export const me = () => request<User>("GET", "/api/me");

// Users
export const listUsers = () => request<Items<PersonUser>>("GET", "/api/users");
export const createUser = (email: string, role: Role) =>
  request<{ user: PersonUser; temporaryPassword: string }>("POST", "/api/users", { email, role });
export const setRole = (id: string, role: Role) =>
  request<PersonUser>("PATCH", `/api/users/${seg(id)}`, { role });
export const deleteUser = (id: string) => request<void>("DELETE", `/api/users/${seg(id)}`);
export const resetPassword = (id: string) =>
  request<{ temporaryPassword: string }>("POST", `/api/users/${seg(id)}/reset-password`);

// Personal access tokens
export const listTokens = () => request<Items<Token>>("GET", "/api/tokens");
export const createToken = (name: string) =>
  request<{ token: Token; secret: string }>("POST", "/api/tokens", { name });
export const deleteToken = (id: string) => request<void>("DELETE", `/api/tokens/${seg(id)}`);

// Channels and keys
export const listChannels = () => request<Items<Channel>>("GET", "/api/channels");
export const createChannel = (name: string) => request<Channel>("POST", "/api/channels", { name });
export const renameChannel = (id: string, name: string) =>
  request<Channel>("PATCH", `/api/channels/${seg(id)}`, { name });
export const deleteChannel = (id: string) => request<void>("DELETE", `/api/channels/${seg(id)}`);
export const listKeys = (channelId?: string) =>
  request<Items<Key>>(
    "GET",
    channelId === undefined ? "/api/keys" : `/api/keys?${new URLSearchParams({ channel: channelId }).toString()}`,
  );
export const createKey = (channelId: string) =>
  request<{ key: Key; secret: string }>("POST", `/api/channels/${seg(channelId)}/keys`);
export const revokeKey = (id: string) => request<Key>("POST", `/api/keys/${seg(id)}/revoke`);

// Feedback
export function listFeedback(filters: Filters, cursor?: string | null, limit?: number) {
  const params = toApiParams(filters);
  if (limit !== undefined) params.set("limit", String(limit));
  if (cursor) params.set("cursor", cursor);
  const qs = params.toString();
  return request<FeedbackPage>("GET", qs ? `/api/feedback?${qs}` : "/api/feedback");
}
export const getFeedback = (id: string) => request<FeedbackDetail>("GET", `/api/feedback/${seg(id)}`);
export const setStatus = (id: string, status: Status) =>
  request<Feedback>("PATCH", `/api/feedback/${seg(id)}`, { status });
export const deleteFeedback = (id: string) => request<void>("DELETE", `/api/feedback/${seg(id)}`);
export const bulkStatus = (ids: string[], status: Status) =>
  request<BulkResult>("POST", "/api/feedback/status", { ids, status });
export const addTags = (ids: string[], tags: string[]) =>
  request<BulkResult>("POST", "/api/feedback/add-tags", { ids, tags });
export const removeTags = (ids: string[], tags: string[]) =>
  request<BulkResult>("POST", "/api/feedback/remove-tags", { ids, tags });
export const deleteByUser = (userId: string) =>
  request<{ deleted: number }>("POST", "/api/feedback/delete-by-user", { userId });
export function exportUrl(filters: Filters, format: "csv" | "json"): string {
  const params = toApiParams(filters);
  params.set("format", format);
  return `/api/feedback/export?${params.toString()}`;
}

// Tags, stats, ops
export const listTags = () => request<Items<TagCount>>("GET", "/api/tags");
/** Counts. The tag, channel, user and search filters are sent when set; the status filter never is. */
export function stats(filters?: Filters) {
  const params = new URLSearchParams();
  if (filters) {
    if (filters.tag !== "") params.set("tag", filters.tag);
    if (filters.channel !== "") params.set("channel", filters.channel);
    if (filters.userId !== "") params.set("userId", filters.userId);
    if (filters.q.trim() !== "") params.set("q", filters.q.trim());
  }
  const qs = params.toString();
  return request<Stats>("GET", qs ? `/api/stats?${qs}` : "/api/stats");
}
export const ops = () => request<Ops>("GET", "/api/ops");
