import { looksLikeEmail } from "../lib/email";
import { AlertCircleIcon } from "./icons";

export function EmailWarning({ userId }: { userId: string | null }) {
  if (userId === null || !looksLikeEmail(userId)) return null;
  return (
    <p className="email-warning">
      <AlertCircleIcon />
      <span>
        This user ID looks like an email address. User IDs should be your internal, non-identifying IDs.
      </span>
    </p>
  );
}
