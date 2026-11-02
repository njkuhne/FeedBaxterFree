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
  it("the last admin's role is plain text with a note", () => {
    setup([person("us_1", "admin@x.test", "admin"), person("us_2", "rev@x.test", "reviewer")], "us_1");
    expect(screen.queryByLabelText("Role for admin@x.test")).toBeNull();
    expect(screen.getByText("Only admin. Make someone else an admin first.")).toBeTruthy();
    expect(screen.getByText("Admin", { selector: "td > span" })).toBeTruthy();
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

  it("a temporary password shows Pending first sign-in", () => {
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer", true)], "us_1");
    expect(screen.getAllByText("Pending first sign-in")).toHaveLength(1);
    expect(screen.queryByText("Temporary password")).toBeNull();
  });

  it("a role change asks for confirmation before saving", async () => {
    const updated = person("us_2", "b@x.test", "admin");
    vi.mocked(api.setRole).mockResolvedValue(updated);
    const onRoleChanged = vi.fn();
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer")], "us_1", onRoleChanged);
    fireEvent.change(screen.getByLabelText("Role for b@x.test"), { target: { value: "admin" } });
    expect(api.setRole).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Change role" }));
    await waitFor(() => expect(screen.getByText("b@x.test is now an admin.")).toBeTruthy());
    expect(api.setRole).toHaveBeenCalledWith("us_2", "admin");
    expect(onRoleChanged).toHaveBeenCalledWith(updated);
  });

  it("toasts an error and keeps the old role when the change fails", async () => {
    vi.mocked(api.setRole).mockRejectedValue(new ApiError(409, "conflict", "An organisation needs at least one admin."));
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer")], "us_1");
    const select = screen.getByLabelText("Role for b@x.test") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "admin" } });
    fireEvent.click(screen.getByRole("button", { name: "Change role" }));
    await waitFor(() => expect(screen.getByText("An organisation needs at least one admin.")).toBeTruthy());
    expect(select.value).toBe("reviewer");
  });

  it("cancelling a role change keeps the old role", () => {
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "reviewer")], "us_1");
    const select = screen.getByLabelText("Role for b@x.test") as HTMLSelectElement;
    fireEvent.change(select, { target: { value: "admin" } });
    expect(screen.getByRole("dialog", { name: "Make b@x.test an admin?" })).toBeTruthy();
    expect(screen.getByText("Admins can delete feedback and manage channels, keys and people.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(api.setRole).not.toHaveBeenCalled();
    expect(select.value).toBe("reviewer");
  });

  it("warns when you make yourself a reviewer", () => {
    setup([person("us_1", "a@x.test", "admin"), person("us_2", "b@x.test", "admin")], "us_1");
    fireEvent.change(screen.getByLabelText("Role for a@x.test"), { target: { value: "reviewer" } });
    expect(screen.getByRole("dialog", { name: "Make a@x.test a reviewer?" })).toBeTruthy();
    expect(
      screen.getByText("Reviewers can read, triage, tag and export, but cannot delete or manage. You will lose access to admin pages."),
    ).toBeTruthy();
  });
});
