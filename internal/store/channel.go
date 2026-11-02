package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Channel is one row of channels.
type Channel struct {
	ID, TenantID, Name string
	CreatedAt          int64
	LastReceivedAt     *int64 // received_at of the newest feedback; filled by ListChannels only
}

// CreateChannel inserts a channel; the name must be unique within the tenant.
func CreateChannel(ctx context.Context, q Querier, d Dialect, tenantID, name string, now int64) (Channel, error) {
	c := Channel{ID: NewID(PrefixChannel), TenantID: tenantID, Name: name, CreatedAt: now}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO channels (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)"),
		c.ID, c.TenantID, c.Name, c.CreatedAt)
	if isUniqueViolation(err) {
		return Channel{}, fmt.Errorf("store: create channel: %w", ErrConflict)
	}
	if err != nil {
		return Channel{}, fmt.Errorf("store: create channel: %w", err)
	}
	return c, nil
}

const channelCols = "id, tenant_id, name, created_at"

// GetChannel returns ErrNotFound when the channel is not in the tenant.
func GetChannel(ctx context.Context, q Querier, d Dialect, tenantID, id string) (Channel, error) {
	var c Channel
	err := q.QueryRowContext(ctx, d.Rebind(
		"SELECT "+channelCols+" FROM channels WHERE tenant_id = ? AND id = ?"), tenantID, id).
		Scan(&c.ID, &c.TenantID, &c.Name, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("store: get channel: %w", ErrNotFound)
	}
	if err != nil {
		return Channel{}, fmt.Errorf("store: get channel: %w", err)
	}
	return c, nil
}

// ListChannels returns the tenant's channels ordered by created_at, id.
func ListChannels(ctx context.Context, q Querier, d Dialect, tenantID string) ([]Channel, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT c.id, c.tenant_id, c.name, c.created_at, "+
			"(SELECT MAX(f.received_at) FROM feedback f WHERE f.tenant_id = c.tenant_id AND f.channel_id = c.id) "+
			"FROM channels c WHERE c.tenant_id = ? ORDER BY c.created_at, c.id"), tenantID)
	if err != nil {
		return nil, fmt.Errorf("store: list channels: %w", err)
	}
	defer rows.Close()
	var out []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.TenantID, &c.Name, &c.CreatedAt, &c.LastReceivedAt); err != nil {
			return nil, fmt.Errorf("store: list channels: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list channels: %w", err)
	}
	return out, nil
}

// CountChannels returns the number of channels in the tenant.
func CountChannels(ctx context.Context, q Querier, d Dialect, tenantID string) (int, error) {
	return countWhere(ctx, q, d, "count channels", "SELECT COUNT(*) FROM channels WHERE tenant_id = ?", tenantID)
}

// RenameChannel returns ErrNotFound for an unknown channel and ErrConflict
// when the name is taken.
func RenameChannel(ctx context.Context, q Querier, d Dialect, tenantID, id, name string) error {
	err := execOne(ctx, q, d, "rename channel",
		"UPDATE channels SET name = ? WHERE tenant_id = ? AND id = ?", name, tenantID, id)
	if isUniqueViolation(err) {
		return fmt.Errorf("store: rename channel: %w", ErrConflict)
	}
	return err
}

// DeleteChannel removes the channel; its keys cascade, feedback is untouched.
func DeleteChannel(ctx context.Context, q Querier, d Dialect, tenantID, id string) error {
	return execOne(ctx, q, d, "delete channel", "DELETE FROM channels WHERE tenant_id = ? AND id = ?", tenantID, id)
}
