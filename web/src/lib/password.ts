export interface PasswordProblem {
  field: "new" | "confirm";
  message: string;
}

/** Client-side checks that mirror the server's: 10 to 256 characters, and both entries match. */
export function checkNewPassword(next: string, confirm: string): PasswordProblem | null {
  if (next.length < 10 || next.length > 256) return { field: "new", message: "Use 10 to 256 characters." };
  if (next !== confirm) return { field: "confirm", message: "The new passwords do not match." };
  return null;
}
