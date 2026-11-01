package mcp

import (
	"context"
	"errors"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/service"
)

func ptr[T any](v T) *T { return &v }

var (
	readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: ptr(false)}
	// writes is for tools that only change state the caller may change and can
	// be repeated safely; none of them deletes anything.
	writes = &sdk.ToolAnnotations{IdempotentHint: true, DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}
)

// toolErr turns a service error into the error a typed tool handler returns,
// which the SDK packs into a CallToolResult with IsError true and the message
// as text. Never leaks internals.
func (h *handler) toolErr(tool string, err error) error {
	var se *service.Error
	if errors.As(err, &se) && se.Kind != 0 {
		return errors.New(se.Msg)
	}
	h.log.Error("mcp: tool failed", "tool", tool, "err", err)
	return errors.New("Something went wrong.")
}

// parseTime parses an optional RFC 3339 argument.
func parseTime(s, name string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, &service.Error{Kind: service.KindInvalid, Msg: "Use an RFC 3339 date for " + name + ", for example 2026-03-01T00:00:00Z."}
	}
	return &t, nil
}

func parseRange(from, to string) (f, t *time.Time, err error) {
	if f, err = parseTime(from, "from"); err != nil {
		return nil, nil, err
	}
	if t, err = parseTime(to, "to"); err != nil {
		return nil, nil, err
	}
	return f, t, nil
}

func principal(ctx context.Context) ext.Principal {
	p, _ := ext.PrincipalFrom(ctx)
	return p
}

func (h *handler) addTools(srv *sdk.Server) {
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "list_feedback",
		Description: "List feedback, newest first, filtered by status, channel, tag, user ID, date range or search text.",
		Annotations: readOnly,
	}, h.listFeedback)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "get_feedback",
		Description: "Get one feedback item with its metadata, tags and status history.",
		Annotations: readOnly,
	}, h.getFeedback)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "list_tags",
		Description: "List the tags in use and how many feedback items have each.",
		Annotations: readOnly,
	}, h.listTags)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "set_status",
		Description: "Set the status of up to 500 feedback items to new, reviewed, actioned or ignored.",
		Annotations: writes,
	}, h.setStatus)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "add_tags",
		Description: "Add tags to up to 500 feedback items, creating tags that do not exist yet.",
		Annotations: writes,
	}, h.addTags)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "remove_tags",
		Description: "Remove tags from up to 500 feedback items.",
		Annotations: writes,
	}, h.removeTags)
	sdk.AddTool(srv, &sdk.Tool{
		Name:        "stats",
		Description: "Count feedback by status, channel and tag for a date range.",
		Annotations: readOnly,
	}, h.stats)
}

func (h *handler) listFeedback(ctx context.Context, _ *sdk.CallToolRequest, in listFeedbackIn) (*sdk.CallToolResult, listFeedbackOut, error) {
	const tool = "list_feedback"
	from, to, err := parseRange(in.From, in.To)
	if err != nil {
		return nil, listFeedbackOut{}, h.toolErr(tool, err)
	}
	f := service.Filter{
		Statuses: in.Status, ChannelID: in.Channel, Tag: in.Tag, UserID: in.UserID, Q: in.Q, From: from, To: to,
	}
	pg, err := h.svc.ListFeedback(ctx, principal(ctx), f, in.Cursor, in.Limit)
	if err != nil {
		return nil, listFeedbackOut{}, h.toolErr(tool, err)
	}
	out := listFeedbackOut{Items: make([]feedbackOut, len(pg.Items))}
	for i, it := range pg.Items {
		out.Items[i] = newFeedbackOut(it)
	}
	if pg.NextCursor != "" {
		out.NextCursor = &pg.NextCursor
	}
	return nil, out, nil
}

func (h *handler) getFeedback(ctx context.Context, _ *sdk.CallToolRequest, in getFeedbackIn) (*sdk.CallToolResult, detailOut, error) {
	d, err := h.svc.GetFeedback(ctx, principal(ctx), in.ID)
	if err != nil {
		return nil, detailOut{}, h.toolErr("get_feedback", err)
	}
	return nil, newDetailOut(d), nil
}

func (h *handler) listTags(ctx context.Context, _ *sdk.CallToolRequest, _ listTagsIn) (*sdk.CallToolResult, listTagsOut, error) {
	ts, err := h.svc.ListTags(ctx, principal(ctx))
	if err != nil {
		return nil, listTagsOut{}, h.toolErr("list_tags", err)
	}
	out := listTagsOut{Items: make([]tagOut, len(ts))}
	for i, t := range ts {
		out.Items[i] = tagOut{t.ID, t.Name, t.Count}
	}
	return nil, out, nil
}

func (h *handler) setStatus(ctx context.Context, _ *sdk.CallToolRequest, in setStatusIn) (*sdk.CallToolResult, bulkOut, error) {
	res, err := h.svc.SetStatus(ctx, principal(ctx), in.IDs, in.Status)
	if err != nil {
		return nil, bulkOut{}, h.toolErr("set_status", err)
	}
	return nil, newBulkOut(res), nil
}

func (h *handler) addTags(ctx context.Context, _ *sdk.CallToolRequest, in tagsIn) (*sdk.CallToolResult, bulkOut, error) {
	res, err := h.svc.AddTags(ctx, principal(ctx), in.IDs, in.Tags)
	if err != nil {
		return nil, bulkOut{}, h.toolErr("add_tags", err)
	}
	return nil, newBulkOut(res), nil
}

func (h *handler) removeTags(ctx context.Context, _ *sdk.CallToolRequest, in tagsIn) (*sdk.CallToolResult, bulkOut, error) {
	res, err := h.svc.RemoveTags(ctx, principal(ctx), in.IDs, in.Tags)
	if err != nil {
		return nil, bulkOut{}, h.toolErr("remove_tags", err)
	}
	return nil, newBulkOut(res), nil
}

func (h *handler) stats(ctx context.Context, _ *sdk.CallToolRequest, in statsIn) (*sdk.CallToolResult, statsOut, error) {
	from, to, err := parseRange(in.From, in.To)
	if err != nil {
		return nil, statsOut{}, h.toolErr("stats", err)
	}
	st, err := h.svc.Stats(ctx, principal(ctx), from, to)
	if err != nil {
		return nil, statsOut{}, h.toolErr("stats", err)
	}
	return nil, newStatsOut(st), nil
}
