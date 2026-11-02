import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { downloadFile } from "./download";

let names: string[];
beforeEach(() => {
  names = [];
  URL.createObjectURL = vi.fn(() => "blob:x");
  URL.revokeObjectURL = vi.fn();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
    names.push(this.download);
  });
});
afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const answer = (status: number, cd?: string) =>
  vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response("a,b", { status, headers: cd ? { "Content-Disposition": cd } : {} }))));

describe("downloadFile", () => {
  it("uses the filename from Content-Disposition", async () => {
    answer(200, 'attachment; filename="feedbaxter-2026-03-01.csv"');
    await downloadFile("/api/feedback/export?format=csv");
    expect(names).toEqual(["feedbaxter-2026-03-01.csv"]);
  });
  it("prefers filename* when present", async () => {
    answer(200, "attachment; filename=\"plain.csv\"; filename*=UTF-8''fancy%20name.csv");
    await downloadFile("/x");
    expect(names).toEqual(["fancy name.csv"]);
  });
  it("falls back when there is no header", async () => {
    answer(200);
    await downloadFile("/api/feedback/export?format=json");
    expect(names).toEqual(["feedbaxter-export.json"]);
  });
  it("throws on a failed response", async () => {
    answer(500);
    await expect(downloadFile("/x")).rejects.toThrow();
    expect(names).toEqual([]);
  });
});
