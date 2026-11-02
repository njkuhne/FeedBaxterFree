import { describe, expect, it } from "vitest";
import { versionText } from "./versionText";

describe("versionText", () => {
  it("release build", () => {
    expect(versionText({ version: "0.4.0", mode: "all", dialect: "sqlite" })).toBe("FeedBaxter 0.4.0 · single process · SQLite");
    expect(versionText({ version: "v0.4.0", mode: "all", dialect: "sqlite" })).toBe("FeedBaxter 0.4.0 · single process · SQLite");
  });
  it("dirty build is a development build", () => {
    expect(versionText({ version: "0.4.0-3-gab12cd-dirty", mode: "all", dialect: "sqlite" })).toBe(
      "FeedBaxter development build (0.4.0-3-gab12cd-dirty) · single process · SQLite",
    );
  });
  it("dev build", () => {
    expect(versionText({ version: "dev", mode: "api", dialect: "postgres" })).toBe("FeedBaxter development build · API process · Postgres");
    expect(versionText({ version: "", mode: "api", dialect: "postgres" })).toBe("FeedBaxter development build · API process · Postgres");
  });
  it("API process mode", () => {
    expect(versionText({ version: "1.0.0", mode: "api", dialect: "postgres" })).toBe("FeedBaxter 1.0.0 · API process · Postgres");
    expect(versionText({ version: "1.0.0", mode: "worker", dialect: "sqlite" })).toBe("FeedBaxter 1.0.0 · worker mode · SQLite");
  });
});
