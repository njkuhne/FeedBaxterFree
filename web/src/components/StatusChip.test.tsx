import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { STATUSES } from "../lib/types";
import { StatusChip } from "./StatusChip";

describe("StatusChip", () => {
  const words = { new: "New", reviewed: "Reviewed", actioned: "Actioned", ignored: "Ignored" } as const;
  for (const status of STATUSES) {
    it(`renders ${status} as a word with a hidden marker`, () => {
      const { container } = render(<StatusChip status={status} />);
      expect(screen.getByText(words[status])).toBeTruthy();
      const svg = container.querySelector("svg");
      expect(svg?.getAttribute("aria-hidden")).toBe("true");
    });
  }
});
