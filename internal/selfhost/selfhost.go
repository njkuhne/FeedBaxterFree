// Package selfhost holds the single-tenant implementations of the ext
// interfaces and the first-boot bootstrap.
package selfhost

import (
	"context"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

const (
	// TenantName is the name of the one self-host tenant.
	TenantName = "Self-host"
	// DefaultChannel is the name of the channel created on first boot.
	DefaultChannel = "Default"

	billingMonthStart = 1
)

var _ ext.TenantResolver = TenantResolver{}
var _ ext.LimitsSource = Limits{}

// TenantResolver is the single-tenant ext.TenantResolver.
type TenantResolver struct{ DB *store.DB }

func toExt(t store.Tenant) ext.Tenant {
	return ext.Tenant{ID: t.ID, Name: t.Name, BillingMonthStart: t.BillingMonthStart}
}

// ByID returns the tenant with the given ID.
func (r TenantResolver) ByID(ctx context.Context, id string) (ext.Tenant, error) {
	t, err := store.GetTenant(ctx, r.DB.SQL, r.DB.Dialect, id)
	if err != nil {
		return ext.Tenant{}, err
	}
	return toExt(t), nil
}

// Default returns the one tenant.
func (r TenantResolver) Default(ctx context.Context) (ext.Tenant, error) {
	t, err := store.FirstTenant(ctx, r.DB.SQL, r.DB.Dialect)
	if err != nil {
		return ext.Tenant{}, err
	}
	return toExt(t), nil
}

// Limits is the self-host ext.LimitsSource: the same limits for every tenant.
type Limits struct{ Daily, Monthly int64 }

// LimitsFromConfig takes DAILY_LIMIT and MONTHLY_LIMIT from cfg.
func LimitsFromConfig(cfg config.Config) Limits {
	return Limits{Daily: cfg.DailyLimit, Monthly: cfg.MonthlyLimit}
}

// Limits returns the configured limits.
func (l Limits) Limits(context.Context, string) (ext.Limits, error) {
	return ext.Limits{Daily: l.Daily, Monthly: l.Monthly}, nil
}
