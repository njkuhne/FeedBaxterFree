import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Ops } from "../lib/types";
import { OpsGrid } from "./OpsGrid";

const base: Ops = {
  queue: { depth: 1234, oldestAgeSeconds: 0, dlq: 0 },
  counters: { day: 14, month: 21200, dayKey: "d", monthKey: "m" },
  dbSizeBytes: 1048576,
  version: "0.2.0",
  mode: "all",
  dialect: "sqlite",
};
const at = new Date(2026, 2, 1, 10, 31, 5);

describe("OpsGrid", () => {
  it("renders labels and formatted values", () => {
    render(<OpsGrid ops={base} unavailable={false} updatedAt={at} onRefresh={() => undefined} />);
    for (const label of [
      "Waiting in queue",
      "Oldest waiting",
      "Could not be stored",
      "Accepted today",
      "Accepted this month",
      "Database size",
      "Version",
    ]) {
      expect(screen.getByText(label)).toBeTruthy();
    }
    expect(screen.getByText("1,234")).toBeTruthy();
    expect(screen.getByText("21,200")).toBeTruthy();
    expect(screen.getByText("None")).toBeTruthy();
    expect(screen.getByText("1.0 MB")).toBeTruthy();
    expect(screen.getByText("0.2.0 · all mode · SQLite")).toBeTruthy();
    expect(screen.getByText(/Updated 10:31:05\. Refreshes every 10 seconds\./)).toBeTruthy();
    expect(screen.queryByText(/Signals/)).toBeNull();
    expect(screen.queryByText(/Processing is behind/)).toBeNull();
  });

  it("shows Postgres and a duration for the oldest item", () => {
    render(
      <OpsGrid
        ops={{ ...base, dialect: "postgres", queue: { depth: 3, oldestAgeSeconds: 125, dlq: 0 } }}
        unavailable={false}
        updatedAt={at}
        onRefresh={() => undefined}
      />,
    );
    expect(screen.getByText("2 min 5 s")).toBeTruthy();
    expect(screen.getByText("0.2.0 · all mode · Postgres")).toBeTruthy();
  });

  it("notes a dead-letter count", () => {
    render(
      <OpsGrid ops={{ ...base, queue: { depth: 0, oldestAgeSeconds: 0, dlq: 2 } }} unavailable={false} updatedAt={at} onRefresh={() => undefined} />,
    );
    expect(screen.getByText('Some submissions could not be stored. See "Signals" in OPS.md.')).toBeTruthy();
  });

  it("notes when processing is behind", () => {
    render(
      <OpsGrid ops={{ ...base, queue: { depth: 9, oldestAgeSeconds: 61, dlq: 0 } }} unavailable={false} updatedAt={at} onRefresh={() => undefined} />,
    );
    expect(screen.getByText("Processing is behind. Submissions are safe in the queue.")).toBeTruthy();
  });

  it("replaces the grid with the unavailable state on error and keeps Refresh now", () => {
    render(<OpsGrid ops={base} unavailable updatedAt={at} onRefresh={() => undefined} />);
    expect(screen.getByRole("alert").textContent).toBe("Operations data is unavailable right now. Retrying every 10 seconds.");
    expect(screen.queryByText("Waiting in queue")).toBeNull();
    expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
    expect(screen.getByRole("button", { name: "Refresh now" })).toBeTruthy();
  });
});
