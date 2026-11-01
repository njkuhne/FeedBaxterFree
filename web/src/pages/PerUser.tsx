import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Button } from "../components/Button";
import { ConfirmDialog } from "../components/ConfirmDialog";
import { EmailWarning } from "../components/EmailWarning";
import { PageTitle } from "../components/PageTitle";
import { useToast } from "../components/Toast";
import { deleteByUser, listChannels, listTags } from "../lib/endpoints";
import { defaultFilters } from "../lib/filters";
import { plural } from "../lib/format";
import type { Filters, TagCount } from "../lib/types";
import { ExportMenu } from "../feedback/ExportMenu";
import { FeedbackList } from "../feedback/FeedbackList";
import { clearFeedbackCache } from "../feedback/useFeedbackList";
import { useAuth } from "../shell/AuthProvider";
import { usePageTitle } from "../shell/usePageTitle";

export default function PerUser() {
  const { userId = "" } = useParams();
  const { user } = useAuth();
  const toast = useToast();
  usePageTitle(`Feedback from ${userId}`);
  const filters = useMemo<Filters>(() => ({ ...defaultFilters, status: "all", userId }), [userId]);
  const [channelCount, setChannelCount] = useState(0);
  const [tags, setTags] = useState<TagCount[]>([]);
  const [confirming, setConfirming] = useState(false);
  const [round, setRound] = useState(0);

  useEffect(() => {
    listChannels()
      .then((c) => setChannelCount(c.items.length))
      .catch(() => undefined);
    listTags()
      .then((t) => setTags(t.items))
      .catch(() => undefined);
  }, []);

  return (
    <>
      <div className="page-head">
        <PageTitle>Feedback from {userId}</PageTitle>
        <ExportMenu filters={filters} count={null} />
      </div>
      <EmailWarning userId={userId} />
      <FeedbackList
        key={`${userId}:${round}`}
        filters={filters}
        showChannel={channelCount >= 2}
        tags={tags}
        origin="user"
        originLabel={userId}
        renderEmpty={(ref) => (
          <p className="empty-plain" ref={ref} tabIndex={-1}>
            No feedback from this user.
          </p>
        )}
      />
      {user?.role === "admin" ? (
        <div className="user-actions">
          <Button variant="danger" onClick={() => setConfirming(true)}>
            Delete all feedback for this user
          </Button>
        </div>
      ) : null}
      {confirming ? (
        <ConfirmDialog
          danger
          title={`Delete all feedback from ${userId}?`}
          body="This permanently deletes every feedback item from this user. Deleted feedback still counts toward this month's submission limit."
          confirmLabel="Delete all feedback"
          busyLabel="Deleting…"
          onClose={() => setConfirming(false)}
          onConfirm={async () => {
            const r = await deleteByUser(userId);
            clearFeedbackCache();
            toast.show(`Deleted ${plural(r.deleted, "feedback item", "feedback items")}.`, "success");
            setRound((n) => n + 1);
          }}
        />
      ) : null}
    </>
  );
}
