import { Button } from "../components/Button";
import { absoluteTime, relativeTime, shortDate } from "../lib/time";
import type { Key } from "../lib/types";

interface KeyTableProps {
  channelName: string;
  keys: Key[];
  onRevoke: (key: Key) => void;
}

export function KeyTable({ channelName, keys, onRevoke }: KeyTableProps) {
  if (keys.length === 0) return <p className="soft">No keys yet.</p>;
  const label = `Keys for ${channelName}`;
  return (
    <div className="table-scroll" tabIndex={0} aria-label={label}>
      <table className="table">
        <caption className="visually-hidden">{label}</caption>
        <thead>
          <tr>
            <th scope="col">Key</th>
            <th scope="col">Created</th>
            <th scope="col">Last used</th>
            <th scope="col">Status</th>
            <th scope="col">
              <span className="visually-hidden">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {keys.map((k) => {
            const revoked = k.revokedAt !== null;
            return (
              <tr key={k.id} className={revoked ? "row-revoked" : undefined}>
                <td className="mono">{k.prefix}…</td>
                <td>{shortDate(k.createdAt)}</td>
                {k.lastUsedAt ? (
                  <td title={absoluteTime(k.lastUsedAt)}>{relativeTime(k.lastUsedAt)}</td>
                ) : (
                  <td>Never</td>
                )}
                <td>{revoked ? `Revoked ${shortDate(k.revokedAt ?? k.createdAt)}` : "Active"}</td>
                <td className="actions">
                  {revoked ? null : (
                    <Button variant="danger" onClick={() => onRevoke(k)} aria-label={`Revoke key ${k.prefix}…`}>
                      Revoke
                    </Button>
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
