// Shapes from docs/API.md. Times are RFC 3339 strings; absent values are null.

export type Role = "admin" | "reviewer";
export type Status = "new" | "reviewed" | "actioned" | "ignored";
export const STATUSES: readonly Status[] = ["new", "reviewed", "actioned", "ignored"];

export interface User {
  id: string;
  email: string;
  role: Role;
  mustChangePassword: boolean;
}

export interface PersonUser extends User {
  createdAt: string;
}

export interface Token {
  id: string;
  name: string;
  prefix: string;
  createdAt: string;
  lastUsedAt: string | null;
}

export interface Channel {
  id: string;
  name: string;
  createdAt: string;
}

export interface Key {
  id: string;
  channelId: string;
  prefix: string;
  createdAt: string;
  revokedAt: string | null;
}

export type Json = string | number | boolean | null | Json[] | { [key: string]: Json };

export interface Feedback {
  id: string;
  channelId: string | null;
  channelName: string | null;
  message: string;
  userId: string | null;
  appVersion: string | null;
  platform: string | null;
  metadata: { [key: string]: Json } | null;
  status: Status;
  receivedAt: string;
  updatedAt: string;
  updatedBy: string | null;
  tags: string[];
}

export interface HistoryEntry {
  from: Status;
  to: Status;
  userId: string;
  userEmail: string | null;
  via: "ui" | "mcp";
  at: string;
}

export interface FeedbackDetail extends Feedback {
  history: HistoryEntry[];
}

export interface FeedbackPage {
  items: Feedback[];
  nextCursor: string | null;
}

export interface BulkResult {
  updated: number;
  notFound: string[];
}

export interface TagCount {
  id: string;
  name: string;
  count: number;
}

export interface Stats {
  total: number;
  byStatus: Partial<Record<Status, number>>;
  byChannel: { channelId: string; name: string | null; count: number }[];
  byTag: { name: string; count: number }[];
}

export interface Ops {
  queue: { depth: number; oldestAgeSeconds: number; dlq: number };
  counters: { day: number; month: number; dayKey: string; monthKey: string };
  dbSizeBytes: number;
  version: string;
  mode: string;
  dialect: string;
}

export interface Items<T> {
  items: T[];
}

/** The filter values the inbox and exports share. */
export interface Filters {
  /** A status, or "all" for no status filter. */
  status: Status | "all";
  tag: string;
  channel: string;
  q: string;
  userId: string;
}
