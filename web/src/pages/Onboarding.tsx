import { useEffect, useRef, useState } from "react";
import { BaxterFull } from "../components/Baxter";
import { Button } from "../components/Button";
import { CodeBlock } from "../components/CodeBlock";
import { PageTitle } from "../components/PageTitle";
import { SecretOnce } from "../components/SecretOnce";
import { useToast } from "../components/Toast";
import { errorMessage } from "../lib/api";
import { createKey, listChannels, stats as fetchStats } from "../lib/endpoints";
import type { Stats } from "../lib/types";
import "../styles/feedback.css";
import { useAuth } from "../shell/AuthProvider";

const POLL_MS = 5000;
const LOG_COMMAND = 'docker compose logs feedbaxter | grep "ingest key"';

function curlCommand(origin: string, key: string): string {
  return [
    `curl -X POST ${origin}/v1/feedback \\`,
    `  -H "Authorization: Bearer ${key}" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '{"message": "The export button does nothing on Safari", "userId": "usr_8f2a91"}'`,
  ].join("\n");
}

interface OnboardingProps {
  /** Called with fresh stats once the first feedback has arrived. */
  onReceived: (stats: Stats) => void;
}

export default function Onboarding({ onReceived }: OnboardingProps) {
  const { user } = useAuth();
  const toast = useToast();
  const admin = user?.role === "admin";
  const [secret, setSecret] = useState<string | null>(null);
  const [showSecret, setShowSecret] = useState(false);
  const [creating, setCreating] = useState(false);
  const received = useRef(onReceived);
  received.current = onReceived;

  // Switch to the inbox when the first feedback arrives; only poll while the page is visible.
  useEffect(() => {
    let alive = true;
    const check = () => {
      if (document.visibilityState !== "visible") return;
      fetchStats()
        .then((s) => {
          if (alive && s.total > 0) received.current(s);
        })
        .catch(() => undefined);
    };
    const timer = setInterval(check, POLL_MS);
    document.addEventListener("visibilitychange", check);
    return () => {
      alive = false;
      clearInterval(timer);
      document.removeEventListener("visibilitychange", check);
    };
  }, []);

  const create = async () => {
    setCreating(true);
    try {
      const channels = (await listChannels()).items;
      const target = channels.find((c) => c.name === "Default") ?? channels[0];
      if (!target) throw new Error("no channel");
      const created = await createKey(target.id);
      setSecret(created.secret);
      setShowSecret(true);
    } catch (e) {
      toast.show(errorMessage(e), "error");
    } finally {
      setCreating(false);
    }
  };

  return (
    <>
      <div className="onboard-intro">
        <PageTitle>Send your first feedback</PageTitle>
        <BaxterFull size={128} decorative />
      </div>
      <ol className="steps">
        <li>
          {admin ? (
            <>
              <p>Create an ingest key for the Default channel.</p>
              {secret === null ? (
                <Button variant="primary" loading={creating} loadingLabel="Creating…" onClick={() => void create()}>
                  Create a key
                </Button>
              ) : null}
              {secret !== null && showSecret ? (
                <SecretOnce
                  title="Copy this key now"
                  value={secret}
                  note="It is shown only once. Only a hash is stored."
                  onDone={() => setShowSecret(false)}
                />
              ) : null}
              <p className="muted">Or use the key printed in the server log at first boot:</p>
              <CodeBlock code={LOG_COMMAND} label="Find the key in the log" />
            </>
          ) : (
            <p>Ask an admin for an ingest key.</p>
          )}
        </li>
        <li>
          <p>Send a test message:</p>
          <CodeBlock code={curlCommand(window.location.origin, secret ?? "fbx_your_key")} label="First API call" />
        </li>
      </ol>
      <p role="status" className="waiting">
        <span className="waiting-dot" aria-hidden="true"></span>
        Waiting for your first feedback…
      </p>
    </>
  );
}
