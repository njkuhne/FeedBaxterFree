// Package selfhost holds the single-tenant implementations of the ext
// interfaces and the first-boot bootstrap.
package selfhost

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/store"
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

// Bootstrap creates the tenant, the default channel and one ingest key when
// the database has no tenant, and logs the key once. It does nothing otherwise.
func Bootstrap(ctx context.Context, db *store.DB, keys ext.KeyWriter, log *slog.Logger, now time.Time) (created bool, err error) {
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("selfhost: bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	n, err := store.CountTenants(ctx, tx, db.Dialect)
	if err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	ms := now.UnixMilli()
	tn, err := store.CreateTenant(ctx, tx, db.Dialect, TenantName, billingMonthStart, ms)
	if err != nil {
		return false, err
	}
	ch, err := store.CreateChannel(ctx, tx, db.Dialect, tn.ID, DefaultChannel, ms)
	if err != nil {
		return false, err
	}
	key := store.GenerateIngestKey()
	row, err := store.CreateIngestKey(ctx, tx, db.Dialect, tn.ID, ch.ID, key, ms)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("selfhost: bootstrap: commit: %w", err)
	}

	// The reconcile repairs a failed sync, so this is not fatal.
	if err := keys.SyncKey(ctx, row.ID); err != nil {
		log.Warn("selfhost: key cache sync failed; the periodic reconcile will retry", "err", err)
	}
	log.Info("ingest key for channel " + DefaultChannel + ": " + key)
	return true, nil
}
