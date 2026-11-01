import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
  it("is labelled and toggles", () => {
    function C() {
      const [on, setOn] = useState(false);
      return <Checkbox label="Select all" checked={on} onChange={(e) => setOn(e.target.checked)} />;
    }
    render(<C />);
    const box = screen.getByLabelText("Select all");
    expect((box as HTMLInputElement).checked).toBe(false);
    fireEvent.click(box);
    expect((box as HTMLInputElement).checked).toBe(true);
  });

  it("keeps a hidden label available to assistive technology", () => {
    render(<Checkbox label="Select this feedback" hideLabel checked={false} onChange={() => undefined} />);
    expect(screen.getByLabelText("Select this feedback")).toBeTruthy();
    expect(screen.getByText("Select this feedback").className).toContain("visually-hidden");
  });

  it("sets the indeterminate property", () => {
    render(<Checkbox label="Some" indeterminate checked={false} onChange={() => undefined} />);
    expect((screen.getByLabelText("Some") as HTMLInputElement).indeterminate).toBe(true);
  });
});
