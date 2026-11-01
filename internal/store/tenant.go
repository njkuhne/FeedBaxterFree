package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Tenant is one row of tenants.
type Tenant struct {
	ID, Name          string
	PlanID            *string
	BillingMonthStart int
	CreatedAt         int64
}

const tenantCols = "id, name, plan_id, billing_month_start, created_at"

func scanTenant(row *sql.Row) (Tenant, error) {
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.PlanID, &t.BillingMonthStart, &t.CreatedAt)
	return t, err
}

// CreateTenant inserts a tenant with no plan.
func CreateTenant(ctx context.Context, q Querier, d Dialect, name string, billingMonthStart int, now int64) (Tenant, error) {
	t := Tenant{ID: NewID(PrefixTenant), Name: name, BillingMonthStart: billingMonthStart, CreatedAt: now}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO tenants (id, name, billing_month_start, created_at) VALUES (?, ?, ?, ?)"),
		t.ID, t.Name, t.BillingMonthStart, t.CreatedAt)
	if err != nil {
		return Tenant{}, fmt.Errorf("store: create tenant: %w", err)
	}
	return t, nil
}

// GetTenant returns ErrNotFound when id does not exist.
func GetTenant(ctx context.Context, q Querier, d Dialect, id string) (Tenant, error) {
	t, err := scanTenant(q.QueryRowContext(ctx, d.Rebind("SELECT "+tenantCols+" FROM tenants WHERE id = ?"), id))
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, fmt.Errorf("store: get tenant: %w", ErrNotFound)
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("store: get tenant: %w", err)
	}
	return t, nil
}

// FirstTenant returns the oldest tenant, or ErrNotFound when there are none.
func FirstTenant(ctx context.Context, q Querier, d Dialect) (Tenant, error) {
	t, err := scanTenant(q.QueryRowContext(ctx, d.Rebind(
		"SELECT "+tenantCols+" FROM tenants ORDER BY created_at, id LIMIT 1")))
	if errors.Is(err, sql.ErrNoRows) {
		return Tenant{}, fmt.Errorf("store: first tenant: %w", ErrNotFound)
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("store: first tenant: %w", err)
	}
	return t, nil
}

// CountTenants returns the number of tenants.
func CountTenants(ctx context.Context, q Querier, d Dialect) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, d.Rebind("SELECT COUNT(*) FROM tenants")).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count tenants: %w", err)
	}
	return n, nil
}
