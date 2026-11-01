package store

import (
	"context"
	"fmt"
)

// Channel is one row of channels.
type Channel struct {
	ID, TenantID, Name string
	CreatedAt          int64
}

// CreateChannel inserts a channel; the name must be unique within the tenant.
func CreateChannel(ctx context.Context, q Querier, d Dialect, tenantID, name string, now int64) (Channel, error) {
	c := Channel{ID: NewID(PrefixChannel), TenantID: tenantID, Name: name, CreatedAt: now}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO channels (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)"),
		c.ID, c.TenantID, c.Name, c.CreatedAt)
	if err != nil {
		return Channel{}, fmt.Errorf("store: create channel: %w", err)
	}
	return c, nil
}
