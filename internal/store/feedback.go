package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FeedbackRow is one feedback item to insert. Nil pointers and nil Metadata
// are stored as NULL.
type FeedbackRow struct {
	ID, TenantID, ChannelID, Message string
	UserID, AppVersion, Platform     *string
	Metadata                         []byte // compact JSON object
	IdempotencyKey                   string
	ReceivedAt                       int64 // Unix ms; updated_at gets the same value
}

const (
	feedbackCols       = "id, tenant_id, channel_id, message, user_id, metadata, app_version, platform, idempotency_key, received_at, updated_at"
	feedbackColCount   = 11
	feedbackPGChunk    = 1000
	feedbackInsertTail = " ON CONFLICT (tenant_id, idempotency_key) DO NOTHING"
)

type preparer interface {
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}

func feedbackArgs(r FeedbackRow) []any {
	var meta any
	if r.Metadata != nil {
		meta = string(r.Metadata)
	}
	return []any{r.ID, r.TenantID, r.ChannelID, r.Message, r.UserID, meta,
		r.AppVersion, r.Platform, r.IdempotencyKey, r.ReceivedAt, r.ReceivedAt}
}

// InsertFeedbackBatch inserts rows, skipping any whose (tenant_id,
// idempotency_key) already exists, and returns how many were inserted. The
// caller owns the transaction.
func InsertFeedbackBatch(ctx context.Context, q Querier, d Dialect, rows []FeedbackRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	if d == Postgres {
		return insertFeedbackPG(ctx, q, d, rows)
	}
	return insertFeedbackSingle(ctx, q, d, rows)
}

func insertFeedbackPG(ctx context.Context, q Querier, d Dialect, rows []FeedbackRow) (int, error) {
	one := "(" + strings.TrimSuffix(strings.Repeat("?,", feedbackColCount), ",") + ")"
	total := 0
	for start := 0; start < len(rows); start += feedbackPGChunk {
		chunk := rows[start:min(start+feedbackPGChunk, len(rows))]
		var b strings.Builder
		b.WriteString("INSERT INTO feedback (" + feedbackCols + ") VALUES ")
		args := make([]any, 0, len(chunk)*feedbackColCount)
		for i, r := range chunk {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(one)
			args = append(args, feedbackArgs(r)...)
		}
		b.WriteString(feedbackInsertTail)
		res, err := q.ExecContext(ctx, d.Rebind(b.String()), args...)
		if err != nil {
			return total, fmt.Errorf("store: insert feedback batch: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("store: insert feedback batch: %w", err)
		}
		total += int(n)
	}
	return total, nil
}

func insertFeedbackSingle(ctx context.Context, q Querier, d Dialect, rows []FeedbackRow) (int, error) {
	query := d.Rebind("INSERT INTO feedback (" + feedbackCols + ") VALUES (" +
		strings.TrimSuffix(strings.Repeat("?,", feedbackColCount), ",") + ")" + feedbackInsertTail)
	exec := q.ExecContext
	if p, ok := q.(preparer); ok {
		stmt, err := p.PrepareContext(ctx, query)
		if err != nil {
			return 0, fmt.Errorf("store: insert feedback batch: prepare: %w", err)
		}
		defer stmt.Close()
		exec = func(ctx context.Context, _ string, args ...any) (sql.Result, error) {
			return stmt.ExecContext(ctx, args...)
		}
	}
	total := 0
	for _, r := range rows {
		res, err := exec(ctx, query, feedbackArgs(r)...)
		if err != nil {
			return total, fmt.Errorf("store: insert feedback batch: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("store: insert feedback batch: %w", err)
		}
		total += int(n)
	}
	return total, nil
}

// DeleteFeedbackOlderThan deletes at most limit rows received before
// cutoffMillis, oldest first, and returns how many were deleted.
func DeleteFeedbackOlderThan(ctx context.Context, q Querier, d Dialect, cutoffMillis int64, limit int) (int64, error) {
	res, err := q.ExecContext(ctx, d.Rebind(
		"DELETE FROM feedback WHERE id IN (SELECT id FROM feedback WHERE received_at < ? ORDER BY received_at LIMIT ?)"),
		cutoffMillis, limit)
	if err != nil {
		return 0, fmt.Errorf("store: delete old feedback: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: delete old feedback: %w", err)
	}
	return n, nil
}
