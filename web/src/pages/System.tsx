import { useCallback, useState } from "react";
import { OpsGrid } from "../admin/OpsGrid";
import { usePolling } from "../admin/usePolling";
import { PageTitle } from "../components/PageTitle";
import * as api from "../lib/endpoints";
import type { Ops } from "../lib/types";
import { usePageTitle } from "../shell/usePageTitle";
import "../styles/admin.css";

export default function System() {
  usePageTitle("System");
  const [ops, setOps] = useState<Ops | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

  const poll = useCallback(async () => {
    try {
      const next = await api.ops();
      setOps(next);
      setUnavailable(false);
      setUpdatedAt(new Date());
    } catch {
      setUnavailable(true);
    }
  }, []);
  const { refresh } = usePolling(poll, 10_000);

  return (
    <>
      <PageTitle>System</PageTitle>
      <section aria-labelledby="ops-h">
        <h2 id="ops-h" className="section-title">
          Operations
        </h2>
        <OpsGrid ops={ops} unavailable={unavailable} updatedAt={updatedAt} onRefresh={refresh} />
      </section>
    </>
  );
}
