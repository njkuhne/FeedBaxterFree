package mcp

import (
	"encoding/json"
	"time"

	"github.com/njkuhne/feedbaxter/internal/service"
	"github.com/njkuhne/feedbaxter/internal/store"
)

// timeLayout matches the JSON API: RFC 3339 UTC with milliseconds.
const timeLayout = "2006-01-02T15:04:05.000Z"

func fmtMs(ms int64) string { return time.UnixMilli(ms).UTC().Format(timeLayout) }

// Tool inputs. Optional fields carry omitempty, which makes them optional in
// the schema the SDK infers.

type listFeedbackIn struct {
	Status  []string `json:"status,omitempty" jsonschema:"Statuses to include: new, reviewed, actioned or ignored."`
	Channel string   `json:"channel,omitempty" jsonschema:"Only feedback from this channel ID."`
	Tag     string   `json:"tag,omitempty" jsonschema:"Only feedback with this tag name."`
	UserID  string   `json:"userId,omitempty" jsonschema:"Only feedback from this opaque customer user ID."`
	From    string   `json:"from,omitempty" jsonschema:"Only feedback received at or after this time, as an RFC 3339 date such as 2026-03-01T00:00:00Z."`
	To      string   `json:"to,omitempty" jsonschema:"Only feedback received before this time, as an RFC 3339 date such as 2026-04-01T00:00:00Z."`
	Q       string   `json:"q,omitempty" jsonschema:"Search text, matched case-insensitively inside the feedback message. At most 200 characters."`
	Limit   int      `json:"limit,omitempty" jsonschema:"Page size, 1 to 200. Defaults to 50."`
	Cursor  string   `json:"cursor,omitempty" jsonschema:"The nextCursor from the previous page."`
}

type getFeedbackIn struct {
	ID string `json:"id" jsonschema:"The feedback ID."`
}

type listTagsIn struct{}

type setStatusIn struct {
	IDs    []string `json:"ids" jsonschema:"Feedback IDs, at most 500."`
	Status string   `json:"status" jsonschema:"The new status: new, reviewed, actioned or ignored."`
}

type tagsIn struct {
	IDs  []string `json:"ids" jsonschema:"Feedback IDs, at most 500."`
	Tags []string `json:"tags" jsonschema:"Tag names, 1 to 20 names of 1 to 64 characters each."`
}

type statsIn struct {
	From string `json:"from,omitempty" jsonschema:"Count feedback received at or after this time, as an RFC 3339 date such as 2026-03-01T00:00:00Z."`
	To   string `json:"to,omitempty" jsonschema:"Count feedback received before this time, as an RFC 3339 date such as 2026-04-01T00:00:00Z."`
}

// Tool outputs. They mirror the JSON API's field names and formats.

type feedbackOut struct {
	ID          string   `json:"id"`
	ChannelID   string   `json:"channelId"`
	ChannelName *string  `json:"channelName"`
	Message     string   `json:"message" jsonschema:"The feedback text, plain text with line breaks preserved."`
	UserID      *string  `json:"userId"`
	AppVersion  *string  `json:"appVersion"`
	Platform    *string  `json:"platform"`
	Metadata    any      `json:"metadata" jsonschema:"Custom key-value data sent with the feedback, or null."`
	Status      string   `json:"status"`
	ReceivedAt  string   `json:"receivedAt"`
	UpdatedAt   string   `json:"updatedAt"`
	UpdatedBy   *string  `json:"updatedBy"`
	Tags        []string `json:"tags"`
}

func newFeedbackOut(f store.Feedback) feedbackOut {
	tags := f.Tags
	if tags == nil {
		tags = []string{}
	}
	var meta any
	if len(f.Metadata) > 0 {
		if err := json.Unmarshal(f.Metadata, &meta); err != nil {
			meta = nil
		}
	}
	return feedbackOut{
		ID: f.ID, ChannelID: f.ChannelID, ChannelName: f.ChannelName, Message: f.Message,
		UserID: f.UserID, AppVersion: f.AppVersion, Platform: f.Platform, Metadata: meta,
		Status: f.Status, ReceivedAt: fmtMs(f.ReceivedAt), UpdatedAt: fmtMs(f.UpdatedAt),
		UpdatedBy: f.UpdatedBy, Tags: tags,
	}
}

type listFeedbackOut struct {
	Items      []feedbackOut `json:"items"`
	NextCursor *string       `json:"nextCursor" jsonschema:"Pass this as cursor to get the next page; null on the last page."`
}

type historyOut struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	UserID    string  `json:"userId"`
	UserEmail *string `json:"userEmail"`
	Via       string  `json:"via" jsonschema:"Where the change came from: ui or mcp."`
	At        string  `json:"at"`
}

type detailOut struct {
	feedbackOut
	History []historyOut `json:"history"`
}

func newDetailOut(d service.Detail) detailOut {
	h := make([]historyOut, len(d.History))
	for i, c := range d.History {
		h[i] = historyOut{From: c.From, To: c.To, UserID: c.UserID, UserEmail: c.UserEmail, Via: c.Via, At: fmtMs(c.ChangedAt)}
	}
	return detailOut{feedbackOut: newFeedbackOut(d.Feedback), History: h}
}

type tagOut struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type listTagsOut struct {
	Items []tagOut `json:"items"`
}

type bulkOut struct {
	Updated  int      `json:"updated"`
	NotFound []string `json:"notFound" jsonschema:"IDs that do not exist; they were skipped."`
}

func newBulkOut(r service.BulkResult) bulkOut {
	nf := r.NotFound
	if nf == nil {
		nf = []string{}
	}
	return bulkOut{Updated: r.Updated, NotFound: nf}
}

type channelCountOut struct {
	ChannelID string  `json:"channelId"`
	Name      *string `json:"name"`
	Count     int64   `json:"count"`
}

type tagCountOut struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type statsOut struct {
	Total     int64             `json:"total"`
	ByStatus  map[string]int64  `json:"byStatus"`
	ByChannel []channelCountOut `json:"byChannel"`
	ByTag     []tagCountOut     `json:"byTag"`
}

func newStatsOut(st store.FeedbackStats) statsOut {
	out := statsOut{Total: st.Total, ByStatus: st.ByStatus, ByChannel: []channelCountOut{}, ByTag: []tagCountOut{}}
	if out.ByStatus == nil {
		out.ByStatus = map[string]int64{}
	}
	for _, c := range st.ByChannel {
		out.ByChannel = append(out.ByChannel, channelCountOut{c.ChannelID, c.Name, c.Count})
	}
	for _, t := range st.ByTag {
		out.ByTag = append(out.ByTag, tagCountOut{t.Name, t.Count})
	}
	return out
}
