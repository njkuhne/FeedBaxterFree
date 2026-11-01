import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToastProvider } from "../components/Toast";
import { ApiError } from "../lib/api";
import * as api from "../lib/endpoints";
import type { PersonUser } from "../lib/types";
import { PeopleTable } from "./PeopleTable";

vi.mock("../lib/endpoints");

const person = (id: string, email: string, role: PersonUser["role"], mustChangePassword = false): PersonUser => ({
  id,
  email,
  role,
  mustChangePassword,
  createdAt: "2026-03-01T09:30:00.000Z",
});

function setup(users: PersonUser[], currentUserId: string, onRoleChanged = vi.fn()) {
  render(
    <MemoryRouter>
      <ToastProvider>
        <div id="app-root">
          <PeopleTable
            users={users}
            currentUserId={currentUserId}
            onRoleChanged={onRoleChanged}
            onReset={vi.fn()}
            onRemoved={vi.fn()}
          />
        </div>
      </ToastProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.resetAllMocks();
});

describe("PeopleTable", () => {
  it("disables the last admin's role and hides Remove, with the explanation", () => {
    setup([person("us_1", "admin@x.test", "admin"), person("us_2", "rev@x.test", "reviewer")], "us_1");
    const select = screen.getByLabelText("Role for admin@x.test") as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    const note = screen.getByText("The last admin cannot be removed or made a reviewer.");
    expect(select.getAttribute("aria-describedby")).toContain(note.id);
    expect(screen.getAllByRole("button", { name: "Remove" })).toHaveLength(1);
  });

  it("gives the self row no Reset or Remove, and a link to the profile", () => {
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "admin")], "us_1");
    expect(screen.getByText(/a@x\.test \(you\)/)).toBeTruthy();
    expect(screen.getAllByRole("button", { name: "Reset password" })).toHaveLength(1);
    expect(screen.getAllByRole("button", { name: "Remove" })).toHaveLength(1);
    expect(screen.getByRole("link", { name: "your profile" }).getAttribute("href")).toBe("/profile");
    expect((screen.getByLabelText("Role for a@x.test") as HTMLSelectElement).disabled).toBe(false);
  });

  it("marks people with a temporary password", () => {
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer", true)], "us_1");
    expect(screen.getAllByText("Temporary password")).toHaveLength(1);
  });

  it("changes a role with PATCH and toasts the sentence", async () => {
    const updated = person("us_2", "b@x.test", "admin");
    vi.mocked(api.setRole).mockResolvedValue(updated);
    const onRoleChanged = vi.fn();
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer")], "us_1", onRoleChanged);
    fireEvent.change(screen.getByLabelText("Role for b@x.test"), { target: { value: "admin" } });
    await waitFor(() => expect(screen.getByText("b@x.test is now an admin.")).toBeTruthy());
    expect(api.setRole).toHaveBeenCalledWith("us_2", "admin");
    expect(onRoleChanged).toHaveBeenCalledWith(updated);
  });

  it("toasts an error and keeps the old role when the change fails", async () => {
    vi.mocked(api.setRole).mockRejectedValue(new ApiError(409, "conflict", "An organisation needs at least one admin."));
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer")], "us_1");
    const select = screen.getByLabelText("Role for b@x.test") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "admin" } });
    await waitFor(() => expect(screen.getByText("An organisation needs at least one admin.")).toBeTruthy());
    expect(select.value).toBe("reviewer");
  });
});
