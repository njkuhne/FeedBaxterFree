package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Feedback is one feedback item as the admin inbox reads it.
type Feedback struct {
	ID, TenantID, ChannelID      string
	ChannelName                  *string // nil when the channel was deleted
	Message                      string
	UserID, AppVersion, Platform *string
	Metadata                     json.RawMessage // nil when NULL
	Status                       string
	ReceivedAt, UpdatedAt        int64
	UpdatedBy                    *string
	Tags                         []string // sorted by lower(name); never nil in results
}

// FeedbackFilter narrows ListFeedback and GetFeedbackStats. Zero fields match
// everything.
type FeedbackFilter struct {
	Statuses  []string // OR'd; empty = any
	ChannelID string
	Tag       string // tag name, matched case-insensitively
	UserID    string // exact
	From, To  *int64 // received_at >= From, received_at < To
	Q         string // case-insensitive substring of message
}

// Cursor is the last row of the previous page.
type Cursor struct {
	ReceivedAt int64
	ID         string
}

// placeholders returns n comma-separated question marks.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func stringArgs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// escapeLike prefixes !, % and _ with ! for use with ESCAPE '!'.
func escapeLike(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '!' || r == '%' || r == '_' {
			b.WriteByte('!')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// feedbackWhere builds the WHERE conditions (without the keyword) over the
// table alias f, always starting with the tenant scope. Only ? placeholders
// are generated; every value travels as an argument.
func feedbackWhere(tenantID string, f FeedbackFilter) (string, []any) {
	conds := []string{"f.tenant_id = ?"}
	args := []any{tenantID}
	if len(f.Statuses) > 0 {
		conds = append(conds, "f.status IN ("+placeholders(len(f.Statuses))+")")
		args = append(args, stringArgs(f.Statuses)...)
	}
	if f.ChannelID != "" {
		conds = append(conds, "f.channel_id = ?")
		args = append(args, f.ChannelID)
	}
	if f.Tag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM feedback_tags ft JOIN tags t ON t.id = ft.tag_id "+
			"WHERE ft.feedback_id = f.id AND t.tenant_id = ? AND lower(t.name) = lower(?))")
		args = append(args, tenantID, f.Tag)
	}
	if f.UserID != "" {
		conds = append(conds, "f.user_id = ?")
		args = append(args, f.UserID)
	}
	if f.From != nil {
		conds = append(conds, "f.received_at >= ?")
		args = append(args, *f.From)
	}
	if f.To != nil {
		conds = append(conds, "f.received_at < ?")
		args = append(args, *f.To)
	}
	if f.Q != "" {
		conds = append(conds, "lower(f.message) LIKE lower(?) ESCAPE '!'")
		args = append(args, "%"+escapeLike(f.Q)+"%")
	}
	return strings.Join(conds, " AND "), args
}

// metadata is cast to text so Postgres JSONB and SQLite TEXT read back alike.
const feedbackSelect = "SELECT f.id, f.tenant_id, f.channel_id, c.name, f.message, f.user_id, f.app_version, f.platform, " +
	"CAST(f.metadata AS TEXT), f.status, f.received_at, f.updated_at, f.updated_by " +
	"FROM feedback f LEFT JOIN channels c ON c.id = f.channel_id AND c.tenant_id = f.tenant_id"

func scanFeedback(s scanner) (Feedback, error) {
	var (
		fb   Feedback
		meta sql.NullString
	)
	err := s.Scan(&fb.ID, &fb.TenantID, &fb.ChannelID, &fb.ChannelName, &fb.Message, &fb.UserID, &fb.AppVersion,
		&fb.Platform, &meta, &fb.Status, &fb.ReceivedAt, &fb.UpdatedAt, &fb.UpdatedBy)
	if meta.Valid {
		fb.Metadata = json.RawMessage(meta.String)
	}
	fb.Tags = []string{}
	return fb, err
}

// attachTags fills Tags on every item with one query.
func attachTags(ctx context.Context, q Querier, d Dialect, items []Feedback) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	idx := make(map[string]int, len(items))
	for i, it := range items {
		ids[i] = it.ID
		idx[it.ID] = i
	}
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT ft.feedback_id, t.name FROM feedback_tags ft JOIN tags t ON t.id = ft.tag_id "+
			"WHERE ft.feedback_id IN ("+placeholders(len(ids))+") ORDER BY lower(t.name), t.name"), stringArgs(ids)...)
	if err != nil {
		return fmt.Errorf("store: feedback tags: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var fid, name string
		if err := rows.Scan(&fid, &name); err != nil {
			return fmt.Errorf("store: feedback tags: %w", err)
		}
		i := idx[fid]
		items[i].Tags = append(items[i].Tags, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: feedback tags: %w", err)
	}
	return nil
}

// ListFeedback returns one page, newest first (received_at DESC, id DESC),
// starting after the cursor when given.
func ListFeedback(ctx context.Context, q Querier, d Dialect, tenantID string, f FeedbackFilter, after *Cursor, limit int) ([]Feedback, error) {
	if limit <= 0 {
		return []Feedback{}, nil
	}
	where, args := feedbackWhere(tenantID, f)
	if after != nil {
		where += " AND (f.received_at < ? OR (f.received_at = ? AND f.id < ?))"
		args = append(args, after.ReceivedAt, after.ReceivedAt, after.ID)
	}
	args = append(args, limit)
	rows, err := q.QueryContext(ctx, d.Rebind(
		feedbackSelect+" WHERE "+where+" ORDER BY f.received_at DESC, f.id DESC LIMIT ?"), args...)
	if err != nil {
		return nil, fmt.Errorf("store: list feedback: %w", err)
	}
	defer rows.Close()
	out := []Feedback{}
	for rows.Next() {
		fb, err := scanFeedback(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list feedback: %w", err)
		}
		out = append(out, fb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list feedback: %w", err)
	}
	rows.Close()
	if err := attachTags(ctx, q, d, out); err != nil {
		return nil, fmt.Errorf("store: list feedback: %w", err)
	}
	return out, nil
}

// GetFeedback returns the item with its tags, or ErrNotFound.
func GetFeedback(ctx context.Context, q Querier, d Dialect, tenantID, id string) (Feedback, error) {
	fb, err := scanFeedback(q.QueryRowContext(ctx, d.Rebind(
		feedbackSelect+" WHERE f.tenant_id = ? AND f.id = ?"), tenantID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Feedback{}, fmt.Errorf("store: get feedback: %w", ErrNotFound)
	}
	if err != nil {
		return Feedback{}, fmt.Errorf("store: get feedback: %w", err)
	}
	items := []Feedback{fb}
	if err := attachTags(ctx, q, d, items); err != nil {
		return Feedback{}, fmt.Errorf("store: get feedback: %w", err)
	}
	return items[0], nil
}

// StatusChange is one row of status_changes.
type StatusChange struct {
	ID, FeedbackID, From, To, UserID string
	UserEmail                        *string
	Via                              string
	ChangedAt                        int64
}

// ListStatusChanges returns the item's history ordered by changed_at, id.
func ListStatusChanges(ctx context.Context, q Querier, d Dialect, tenantID, feedbackID string) ([]StatusChange, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT sc.id, sc.feedback_id, sc.from_status, sc.to_status, sc.user_id, u.email, sc.via, sc.changed_at "+
			"FROM status_changes sc LEFT JOIN users u ON u.id = sc.user_id AND u.tenant_id = sc.tenant_id "+
			"WHERE sc.tenant_id = ? AND sc.feedback_id = ? ORDER BY sc.changed_at, sc.id"), tenantID, feedbackID)
	if err != nil {
		return nil, fmt.Errorf("store: list status changes: %w", err)
	}
	defer rows.Close()
	var out []StatusChange
	for rows.Next() {
		var c StatusChange
		if err := rows.Scan(&c.ID, &c.FeedbackID, &c.From, &c.To, &c.UserID, &c.UserEmail, &c.Via, &c.ChangedAt); err != nil {
			return nil, fmt.Errorf("store: list status changes: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list status changes: %w", err)
	}
	return out, nil
}

// SetFeedbackStatus moves the items in ids to status, recording one
// status_changes row per item whose status actually changed. It returns the
// IDs that changed and the IDs that do not exist in the tenant. The caller
// owns the transaction. ids must be deduplicated.
func SetFeedbackStatus(ctx context.Context, q Querier, d Dialect, tenantID string, ids []string, status, userID, via string, now int64) (changed, missing []string, err error) {
	if len(ids) == 0 {
		return nil, nil, nil
	}
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT id, status FROM feedback WHERE tenant_id = ? AND id IN ("+placeholders(len(ids))+")"),
		append([]any{tenantID}, stringArgs(ids)...)...)
	if err != nil {
		return nil, nil, fmt.Errorf("store: set feedback status: %w", err)
	}
	current := make(map[string]string, len(ids))
	for rows.Next() {
		var id, st string
		if err := rows.Scan(&id, &st); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("store: set feedback status: %w", err)
		}
		current[id] = st
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("store: set feedback status: %w", err)
	}
	for _, id := range ids {
		st, ok := current[id]
		switch {
		case !ok:
			missing = append(missing, id)
		case st != status:
			changed = append(changed, id)
		}
	}
	if len(changed) == 0 {
		return nil, missing, nil
	}
	args := append([]any{status, now, userID, tenantID}, stringArgs(changed)...)
	args = append(args, status)
	if _, err := q.ExecContext(ctx, d.Rebind(
		"UPDATE feedback SET status = ?, updated_at = ?, updated_by = ? WHERE tenant_id = ? AND id IN ("+
			placeholders(len(changed))+") AND status <> ?"), args...); err != nil {
		return nil, nil, fmt.Errorf("store: set feedback status: %w", err)
	}
	var b strings.Builder
	b.WriteString("INSERT INTO status_changes (id, tenant_id, feedback_id, from_status, to_status, user_id, via, changed_at) VALUES ")
	hargs := make([]any, 0, len(changed)*8)
	for i, id := range changed {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("(" + placeholders(8) + ")")
		hargs = append(hargs, NewID(PrefixStatusChange), tenantID, id, current[id], status, userID, via, now)
	}
	if _, err := q.ExecContext(ctx, d.Rebind(b.String()), hargs...); err != nil {
		return nil, nil, fmt.Errorf("store: set feedback status: %w", err)
	}
	return changed, missing, nil
}

// Tag is a tag with the number of feedback items carrying it.
type Tag struct {
	ID, Name string
	Count    int64
}

// EnsureTags returns the tags for names, creating missing ones. Matching is
// case-insensitive; an existing tag keeps its stored spelling; names that
// differ only in case collapse. The result is sorted by lower(name).
func EnsureTags(ctx context.Context, q Querier, d Dialect, tenantID string, names []string, now int64) ([]Tag, error) {
	seen := make(map[string]bool, len(names))
	var uniq []string
	for _, n := range names {
		k := strings.ToLower(n)
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, n)
		}
	}
	if len(uniq) == 0 {
		return nil, nil
	}
	lookup := func() ([]Tag, error) {
		rows, err := q.QueryContext(ctx, d.Rebind(
			"SELECT id, name FROM tags WHERE tenant_id = ? AND lower(name) IN ("+lowerPlaceholders(len(uniq))+") ORDER BY lower(name), name"),
			append([]any{tenantID}, stringArgs(uniq)...)...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Tag
		for rows.Next() {
			var t Tag
			if err := rows.Scan(&t.ID, &t.Name); err != nil {
				return nil, err
			}
			out = append(out, t)
		}
		return out, rows.Err()
	}
	found, err := lookup()
	if err != nil {
		return nil, fmt.Errorf("store: ensure tags: %w", err)
	}
	if len(found) == len(uniq) {
		return found, nil
	}
	have := make(map[string]bool, len(found))
	for _, t := range found {
		have[strings.ToLower(t.Name)] = true
	}
	for _, n := range uniq {
		if have[strings.ToLower(n)] {
			continue
		}
		// DO NOTHING (any unique index) so a concurrent creator cannot make
		// this fail and, on Postgres, abort the caller's transaction.
		if _, err := q.ExecContext(ctx, d.Rebind(
			"INSERT INTO tags (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING"),
			NewID(PrefixTag), tenantID, n, now); err != nil {
			return nil, fmt.Errorf("store: ensure tags: %w", err)
		}
	}
	out, err := lookup()
	if err != nil {
		return nil, fmt.Errorf("store: ensure tags: %w", err)
	}
	return out, nil
}

// lowerPlaceholders returns n comma-separated lower(?) expressions.
func lowerPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("lower(?),", n), ",")
}

// missingIDs returns the IDs (in input order) not present in the tenant.
func missingIDs(ctx context.Context, q Querier, d Dialect, tenantID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT id FROM feedback WHERE tenant_id = ? AND id IN ("+placeholders(len(ids))+")"),
		append([]any{tenantID}, stringArgs(ids)...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	present := make(map[string]bool, len(ids))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		present[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var missing []string
	for _, id := range ids {
		if !present[id] {
			missing = append(missing, id)
		}
	}
	return missing, nil
}

// AddFeedbackTags links each tag to each existing item in ids (idempotent)
// and returns the IDs not found in the tenant.
func AddFeedbackTags(ctx context.Context, q Querier, d Dialect, tenantID string, ids, tagIDs []string) (missing []string, err error) {
	missing, err = missingIDs(ctx, q, d, tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("store: add feedback tags: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	for _, tagID := range tagIDs {
		// The tags subquery keeps a foreign tag ID from being linked.
		args := append([]any{tenantID, tagID, tenantID}, stringArgs(ids)...)
		args = append(args, tagID, tenantID)
		if _, err := q.ExecContext(ctx, d.Rebind(
			"INSERT INTO feedback_tags (tenant_id, feedback_id, tag_id) SELECT ?, f.id, ? FROM feedback f "+
				"WHERE f.tenant_id = ? AND f.id IN ("+placeholders(len(ids))+") "+
				"AND EXISTS (SELECT 1 FROM tags t WHERE t.id = ? AND t.tenant_id = ?) ON CONFLICT DO NOTHING"), args...); err != nil {
			return nil, fmt.Errorf("store: add feedback tags: %w", err)
		}
	}
	return missing, nil
}

// RemoveFeedbackTags unlinks the tags named names (case-insensitive) from
// the items in ids and returns the IDs not found in the tenant. The tags
// themselves stay.
func RemoveFeedbackTags(ctx context.Context, q Querier, d Dialect, tenantID string, ids, names []string) (missing []string, err error) {
	missing, err = missingIDs(ctx, q, d, tenantID, ids)
	if err != nil {
		return nil, fmt.Errorf("store: remove feedback tags: %w", err)
	}
	if len(ids) == 0 || len(names) == 0 {
		return missing, nil
	}
	args := append([]any{tenantID}, stringArgs(ids)...)
	args = append(args, tenantID)
	args = append(args, stringArgs(names)...)
	if _, err := q.ExecContext(ctx, d.Rebind(
		"DELETE FROM feedback_tags WHERE tenant_id = ? AND feedback_id IN ("+placeholders(len(ids))+") "+
			"AND tag_id IN (SELECT id FROM tags WHERE tenant_id = ? AND lower(name) IN ("+lowerPlaceholders(len(names))+"))"),
		args...); err != nil {
		return nil, fmt.Errorf("store: remove feedback tags: %w", err)
	}
	return missing, nil
}

// ListTags returns every tag with its link count (zero allowed), ordered by
// lower(name).
func ListTags(ctx context.Context, q Querier, d Dialect, tenantID string) ([]Tag, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT t.id, t.name, COUNT(ft.feedback_id) FROM tags t LEFT JOIN feedback_tags ft ON ft.tag_id = t.id "+
			"WHERE t.tenant_id = ? GROUP BY t.id, t.name ORDER BY lower(t.name), t.name"), tenantID)
	if err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	defer rows.Close()
	var out []Tag
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return nil, fmt.Errorf("store: list tags: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list tags: %w", err)
	}
	return out, nil
}

// FeedbackStats are counts over a date range.
type FeedbackStats struct {
	Total     int64
	ByStatus  map[string]int64
	ByChannel []ChannelCount
	ByTag     []Tag
}

// ChannelCount is the number of items in a channel; Name is nil when the
// channel was deleted.
type ChannelCount struct {
	ChannelID string
	Name      *string
	Count     int64
}

// GetFeedbackStats counts feedback received in [from, to) by status,
// channel and tag. ByStatus always has all four keys.
func GetFeedbackStats(ctx context.Context, q Querier, d Dialect, tenantID string, from, to *int64) (FeedbackStats, error) {
	where, args := feedbackWhere(tenantID, FeedbackFilter{From: from, To: to})
	st := FeedbackStats{ByStatus: map[string]int64{"new": 0, "reviewed": 0, "actioned": 0, "ignored": 0}}

	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT f.status, COUNT(*) FROM feedback f WHERE "+where+" GROUP BY f.status"), args...)
	if err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}
	for rows.Next() {
		var s string
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			rows.Close()
			return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
		}
		st.ByStatus[s] = n
		st.Total += n
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}

	rows, err = q.QueryContext(ctx, d.Rebind(
		"SELECT f.channel_id, c.name, COUNT(*) FROM feedback f "+
			"LEFT JOIN channels c ON c.id = f.channel_id AND c.tenant_id = f.tenant_id WHERE "+where+
			" GROUP BY f.channel_id, c.name ORDER BY COUNT(*) DESC, f.channel_id"), args...)
	if err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}
	for rows.Next() {
		var c ChannelCount
		if err := rows.Scan(&c.ChannelID, &c.Name, &c.Count); err != nil {
			rows.Close()
			return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
		}
		st.ByChannel = append(st.ByChannel, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}

	rows, err = q.QueryContext(ctx, d.Rebind(
		"SELECT t.id, t.name, COUNT(*) FROM feedback f JOIN feedback_tags ft ON ft.feedback_id = f.id "+
			"JOIN tags t ON t.id = ft.tag_id WHERE "+where+
			" GROUP BY t.id, t.name ORDER BY COUNT(*) DESC, lower(t.name), t.name"), args...)
	if err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Count); err != nil {
			return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
		}
		st.ByTag = append(st.ByTag, t)
	}
	if err := rows.Err(); err != nil {
		return FeedbackStats{}, fmt.Errorf("store: feedback stats: %w", err)
	}
	return st, nil
}

// DeleteFeedback removes one item; tags and history cascade.
func DeleteFeedback(ctx context.Context, q Querier, d Dialect, tenantID, id string) error {
	return execOne(ctx, q, d, "delete feedback", "DELETE FROM feedback WHERE tenant_id = ? AND id = ?", tenantID, id)
}

// DeleteFeedbackByUser removes every item from the end user and returns how
// many were deleted.
func DeleteFeedbackByUser(ctx context.Context, q Querier, d Dialect, tenantID, userID string) (int64, error) {
	return execCount(ctx, q, d, "delete feedback by user",
		"DELETE FROM feedback WHERE tenant_id = ? AND user_id = ?", tenantID, userID)
}
