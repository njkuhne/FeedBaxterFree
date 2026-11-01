import { DownloadIcon } from "../components/icons";
import { exportUrl } from "../lib/endpoints";
import type { Filters } from "../lib/types";

export function ExportLinks({ filters }: { filters: Filters }) {
  return (
    <div className="export-links">
      <a className="btn btn-text" href={exportUrl(filters, "csv")} download>
        <DownloadIcon />
        Export CSV
      </a>
      <a className="btn btn-text" href={exportUrl(filters, "json")} download>
        <DownloadIcon />
        Export JSON
      </a>
    </div>
  );
}
