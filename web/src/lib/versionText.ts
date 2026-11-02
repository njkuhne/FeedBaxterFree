import type { Ops } from "./types";

/** The version line under the System grid, in plain words. */
export function versionText(ops: Pick<Ops, "version" | "mode" | "dialect">): string {
  const raw = ops.version.trim().replace(/^v/, "");
  const dev = raw === "" || raw === "dev" || raw.includes("-dirty");
  const name = dev ? (raw === "" || raw === "dev" ? "FeedBaxter development build" : `FeedBaxter development build (${raw})`) : `FeedBaxter ${raw}`;
  const mode = ops.mode === "all" ? "single process" : ops.mode === "api" ? "API process" : `${ops.mode} mode`;
  const db = ops.dialect === "postgres" ? "Postgres" : "SQLite";
  return `${name} · ${mode} · ${db}`;
}
