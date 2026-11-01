// Package feedbaxter is the single wiring entry point of FeedBaxter: Run starts
// the configured mode and Migrate applies the schema.
package feedbaxter

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/store"
)

// Version is the build version, set by -ldflags "-X github.com/njkuhne/feedbaxter.Version=<v>".
var Version = "dev"

// ErrNotImplemented is returned by functionality that lands in a later milestone.
var ErrNotImplemented = errors.New("not implemented")

// Options is what a binary (the public cmd or the private SaaS repo) fills in to
// customise FeedBaxter.
type Options struct {
	// Mode overrides MODE when non-empty: ingest, worker, api or all.
	Mode string
	// TenantResolver maps principals and keys to tenants.
	TenantResolver ext.TenantResolver
	// Authenticator produces session principals for the api mode.
	Authenticator ext.Authenticator
	// LimitsSource decides per-tenant daily and monthly limits.
	LimitsSource ext.LimitsSource
	// Migrations are extra migrations applied after the core ones.
	Migrations []ext.Migration
	// MountRoutes registers extra HTTP routes.
	MountRoutes func(mux *http.ServeMux, deps ext.Deps)
	// OnStart hooks run once the dependencies are ready.
	OnStart []func(ctx context.Context, deps ext.Deps) error
}

// Run starts the configured mode and blocks until ctx is done.
func Run(ctx context.Context, opts Options) error {
	overrides := map[string]string{}
	if opts.Mode != "" {
		overrides["MODE"] = opts.Mode
	}
	if _, err := config.Load(overrides); err != nil {
		return err
	}
	return fmt.Errorf("serve: %w until M1", ErrNotImplemented)
}

// Migrate applies core migrations then opts.Migrations, and returns the versions it applied.
func Migrate(ctx context.Context, opts Options) ([]string, error) {
	cfg, err := config.Load(nil)
	if err != nil {
		return nil, err
	}
	log := cfg.Logger(os.Stderr)
	db, err := store.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	applied, err := store.Migrate(ctx, db, opts.Migrations)
	if err != nil {
		return nil, err
	}
	log.Info("migrations applied", "count", len(applied), "versions", applied)
	return applied, nil
}
