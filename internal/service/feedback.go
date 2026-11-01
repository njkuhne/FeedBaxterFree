package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

const (
	// MaxBulkIDs is the most feedback IDs one bulk call accepts.
	MaxBulkIDs = 500
	// MaxLimit is the largest page size.
	MaxLimit = 200
	// DefaultLimit is the page size when none is given.
	DefaultLimit = 50

	maxTagsPerCall = 20
	maxQLen        = 200
	exportPageSize = 500
)

var validStatuses = map[string]bool{"new": true, "reviewed": true, "actioned": true, "ignored": true}

// Filter narrows a feedback list, export or stats call.
type Filter struct {
	Statuses                  []string
	ChannelID, Tag, UserID, Q string
	From, To                  *time.Time
}

// Page is one page of feedback; an empty NextCursor means no more.
type Page struct {
	Items      []store.Feedback
	NextCursor string
}

// Detail is one feedback item with its status history.
type Detail struct {
	store.Feedback
	History []store.StatusChange
}

// BulkResult reports a bulk call. NotFound is never nil.
type BulkResult struct {
	Updated  int
	NotFound []string
}

func validateRange(from, to *time.Time) error {
	if from != nil && to != nil && from.After(*to) {
		return invalid("The start date must not be after the end date.")
	}
	return nil
}

func (f Filter) toStore() (store.FeedbackFilter, error) {
	for _, st := range f.Statuses {
		if !validStatuses[st] {
			return store.FeedbackFilter{}, invalid("Unknown status.")
		}
	}
	if len([]rune(f.Q)) > maxQLen {
		return store.FeedbackFilter{}, invalid("Search text is too long.")
	}
	if err := validateRange(f.From, f.To); err != nil {
		return store.FeedbackFilter{}, err
	}
	sf := store.FeedbackFilter{
		Statuses: f.Statuses, ChannelID: f.ChannelID, Tag: f.Tag, UserID: f.UserID, Q: f.Q,
	}
	sf.From, sf.To = millis(f.From), millis(f.To)
	return sf, nil
}

func millis(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

type cursorJSON struct {
	R int64  `json:"r"`
	I string `json:"i"`
}

func encodeCursor(fb store.Feedback) string {
	b, _ := json.Marshal(cursorJSON{R: fb.ReceivedAt, I: fb.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(c string) (*store.Cursor, error) {
	if c == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, invalid("Invalid cursor.")
	}
	var cj cursorJSON
	if err := json.Unmarshal(raw, &cj); err != nil || cj.I == "" {
		return nil, invalid("Invalid cursor.")
	}
	return &store.Cursor{ReceivedAt: cj.R, ID: cj.I}, nil
}

// ListFeedback returns one page of feedback, newest first.
func (s *Service) ListFeedback(ctx context.Context, p ext.Principal, f Filter, cursor string, limit int) (Page, error) {
	if err := requireUser(p); err != nil {
		return Page{}, err
	}
	if limit < 0 || limit > MaxLimit {
		return Page{}, invalid("Limit must be between 1 and 200.")
	}
	if limit == 0 {
		limit = DefaultLimit
	}
	sf, err := f.toStore()
	if err != nil {
		return Page{}, err
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}
	return s.page(ctx, p.TenantID, sf, after, limit)
}

func (s *Service) page(ctx context.Context, tenantID string, sf store.FeedbackFilter, after *store.Cursor, limit int) (Page, error) {
	items, err := store.ListFeedback(ctx, s.DB.SQL, s.d(), tenantID, sf, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	pg := Page{Items: items}
	if len(items) > limit {
		pg.Items = items[:limit]
		pg.NextCursor = encodeCursor(items[limit-1])
	}
	return pg, nil
}

// GetFeedback returns one item with its status history.
func (s *Service) GetFeedback(ctx context.Context, p ext.Principal, id string) (Detail, error) {
	if err := requireUser(p); err != nil {
		return Detail{}, err
	}
	fb, err := store.GetFeedback(ctx, s.DB.SQL, s.d(), p.TenantID, id)
	if err != nil {
		return Detail{}, mapErr(err, "Feedback not found.", "")
	}
	h, err := store.ListStatusChanges(ctx, s.DB.SQL, s.d(), p.TenantID, id)
	if err != nil {
		return Detail{}, err
	}
	if h == nil {
		h = []store.StatusChange{}
	}
	return Detail{Feedback: fb, History: h}, nil
}

// normaliseIDs drops duplicates (keeping order) and enforces 1..MaxBulkIDs.
func normaliseIDs(ids []string) ([]string, error) {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, invalid("Choose at least one feedback item.")
	}
	if len(out) > MaxBulkIDs {
		return nil, invalid("Choose at most 500 feedback items at a time.")
	}
	return out, nil
}

func normaliseTagNames(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if l := len([]rune(n)); l < 1 || l > maxNameLen {
			return nil, invalid("Tag names must be 1 to 64 characters.")
		}
		out = append(out, n)
	}
	if len(out) == 0 || len(out) > maxTagsPerCall {
		return nil, invalid("Give 1 to 20 tags.")
	}
	return out, nil
}

func bulk(ids, missing []string) BulkResult {
	if missing == nil {
		missing = []string{}
	}
	return BulkResult{Updated: len(ids) - len(missing), NotFound: missing}
}

// SetStatus moves feedback to a status; p.Via records where the change came from.
// Updated counts the items found, including ones already in that status.
func (s *Service) SetStatus(ctx context.Context, p ext.Principal, ids []string, status string) (BulkResult, error) {
	if err := requireUser(p); err != nil {
		return BulkResult{}, err
	}
	if p.Via != "ui" && p.Via != "mcp" {
		return BulkResult{}, invalid("Unknown request source.")
	}
	if !validStatuses[status] {
		return BulkResult{}, invalid("Unknown status.")
	}
	ids, err := normaliseIDs(ids)
	if err != nil {
		return BulkResult{}, err
	}
	var missing []string
	err = s.tx(ctx, func(q store.Querier) error {
		var err error
		_, missing, err = store.SetFeedbackStatus(ctx, q, s.d(), p.TenantID, ids, status, p.UserID, p.Via, s.nowMs())
		return err
	})
	if err != nil {
		return BulkResult{}, err
	}
	return bulk(ids, missing), nil
}

// AddTags tags feedback, creating tags as needed.
func (s *Service) AddTags(ctx context.Context, p ext.Principal, ids, names []string) (BulkResult, error) {
	if err := requireUser(p); err != nil {
		return BulkResult{}, err
	}
	ids, err := normaliseIDs(ids)
	if err != nil {
		return BulkResult{}, err
	}
	names, err = normaliseTagNames(names)
	if err != nil {
		return BulkResult{}, err
	}
	var missing []string
	err = s.tx(ctx, func(q store.Querier) error {
		tags, err := store.EnsureTags(ctx, q, s.d(), p.TenantID, names, s.nowMs())
		if err != nil {
			return err
		}
		tagIDs := make([]string, len(tags))
		for i, t := range tags {
			tagIDs[i] = t.ID
		}
		missing, err = store.AddFeedbackTags(ctx, q, s.d(), p.TenantID, ids, tagIDs)
		return err
	})
	if err != nil {
		return BulkResult{}, err
	}
	return bulk(ids, missing), nil
}

// RemoveTags unlinks tags from feedback; the tags themselves stay.
func (s *Service) RemoveTags(ctx context.Context, p ext.Principal, ids, names []string) (BulkResult, error) {
	if err := requireUser(p); err != nil {
		return BulkResult{}, err
	}
	ids, err := normaliseIDs(ids)
	if err != nil {
		return BulkResult{}, err
	}
	names, err = normaliseTagNames(names)
	if err != nil {
		return BulkResult{}, err
	}
	var missing []string
	err = s.tx(ctx, func(q store.Querier) error {
		var err error
		missing, err = store.RemoveFeedbackTags(ctx, q, s.d(), p.TenantID, ids, names)
		return err
	})
	if err != nil {
		return BulkResult{}, err
	}
	return bulk(ids, missing), nil
}

// ListTags returns the tenant's tags with their counts.
func (s *Service) ListTags(ctx context.Context, p ext.Principal) ([]store.Tag, error) {
	if err := requireUser(p); err != nil {
		return nil, err
	}
	ts, err := store.ListTags(ctx, s.DB.SQL, s.d(), p.TenantID)
	if ts == nil {
		ts = []store.Tag{}
	}
	return ts, err
}

// Stats counts feedback received in [from, to).
func (s *Service) Stats(ctx context.Context, p ext.Principal, from, to *time.Time) (store.FeedbackStats, error) {
	if err := requireUser(p); err != nil {
		return store.FeedbackStats{}, err
	}
	if err := validateRange(from, to); err != nil {
		return store.FeedbackStats{}, err
	}
	return store.GetFeedbackStats(ctx, s.DB.SQL, s.d(), p.TenantID, millis(from), millis(to))
}

// DeleteFeedback deletes one item (admin only). Counters are never touched:
// deleted feedback still counts toward the month's limit.
func (s *Service) DeleteFeedback(ctx context.Context, p ext.Principal, id string) error {
	if err := requireAdmin(p); err != nil {
		return err
	}
	return mapErr(store.DeleteFeedback(ctx, s.DB.SQL, s.d(), p.TenantID, id), "Feedback not found.", "")
}

// DeleteUserFeedback deletes every item from one end user and returns how
// many (admin only). Counters are never touched.
func (s *Service) DeleteUserFeedback(ctx context.Context, p ext.Principal, userID string) (int64, error) {
	if err := requireAdmin(p); err != nil {
		return 0, err
	}
	if userID == "" {
		return 0, invalid("Give the user ID to delete feedback for.")
	}
	return store.DeleteFeedbackByUser(ctx, s.DB.SQL, s.d(), p.TenantID, userID)
}

// Export calls fn for every matching item, newest first, one page of 500 at a
// time so memory stays bounded. It stops on an fn error or a done context.
func (s *Service) Export(ctx context.Context, p ext.Principal, f Filter, fn func(store.Feedback) error) error {
	if err := requireUser(p); err != nil {
		return err
	}
	sf, err := f.toStore()
	if err != nil {
		return err
	}
	var after *store.Cursor
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		items, err := store.ListFeedback(ctx, s.DB.SQL, s.d(), p.TenantID, sf, after, exportPageSize)
		if err != nil {
			return err
		}
		for _, it := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := fn(it); err != nil {
				return err
			}
		}
		if len(items) < exportPageSize {
			return nil
		}
		last := items[len(items)-1]
		after = &store.Cursor{ReceivedAt: last.ReceivedAt, ID: last.ID}
	}
}
