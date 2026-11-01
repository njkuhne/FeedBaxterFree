import type { Status } from "../lib/types";
import { CheckCircleIcon, DotIcon, EyeIcon, MinusCircleIcon } from "./icons";

/**
 * 16 px status icon. Decorative when the status word is next to it. Pass `label` when the word is not
 * shown: the marker then becomes an image with that accessible name.
 */
export function StatusMarker({ status, label }: { status: Status; label?: string }) {
  const marker = (() => {
    switch (status) {
      case "new":
        return <DotIcon className="marker marker-new" />;
      case "reviewed":
        return <EyeIcon className="marker marker-reviewed" />;
      case "actioned":
        return <CheckCircleIcon className="marker marker-actioned" />;
      case "ignored":
        return <MinusCircleIcon className="marker marker-ignored" />;
    }
  })();
  if (label === undefined) return marker;
  return (
    <span className="marker-label" role="img" aria-label={label}>
      {marker}
    </span>
  );
}

export const STATUS_LABEL: Record<Status, string> = {
  new: "New",
  reviewed: "Reviewed",
  actioned: "Actioned",
  ignored: "Ignored",
};
