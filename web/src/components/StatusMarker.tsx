import type { Status } from "../lib/types";
import { CheckCircleIcon, DotIcon, EyeIcon, MinusCircleIcon } from "./icons";

/** 16 px status icon. Decorative: the status word is always next to it. */
export function StatusMarker({ status }: { status: Status }) {
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
}

export const STATUS_LABEL: Record<Status, string> = {
  new: "New",
  reviewed: "Reviewed",
  actioned: "Actioned",
  ignored: "Ignored",
};
