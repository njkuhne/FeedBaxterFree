// Package feedbaxter is the single wiring entry point of FeedBaxter: Run starts
// the configured mode and Migrate applies the schema.
package feedbaxter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/api"
	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/ingest"
	"github.com/njkuhne/feedbaxterfree/internal/ingest/keysync"
	"github.com/njkuhne/feedbaxterfree/internal/mcp"
	"github.com/njkuhne/feedbaxterfree/internal/processor"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/selfhost"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/ui"
	"github.com/njkuhne/feedbaxterfree/web"
)

// Version is the build version, set by -ldflags "-X github.com/njkuhne/feedbaxterfree.Version=<v>".
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

// logOutput is where Run logs; tests replace it.
var logOutput io.Writer = os.Stderr

// Run starts the configured mode and blocks until ctx is done or a component
// fails. On the way out it stops the HTTP server first, then the components,
// then the queue, then the database. It returns nil after a clean ctx shutdown.
func Run(ctx context.Context, opts Options) (err error) {
	overrides := map[string]string{}
	if opts.Mode != "" {
		overrides["MODE"] = opts.Mode
	}
	cfg, err := config.Load(overrides)
	if err != nil {
		return err
	}
	log := cfg.Logger(logOutput)
	needDB := cfg.Mode != "ingest" // ingest never opens a database
	runsIngest := cfg.Mode == "ingest" || cfg.Mode == "all"
	runsProcessor := cfg.Mode == "worker" || cfg.Mode == "all"
	writesKeys := cfg.Mode == "api" || cfg.Mode == "all"
	selfHost := opts.TenantResolver == nil

	// Deferred first so they run last: queue Close, then DB Close.
	var db *store.DB
	var conn *queue.Conn
	defer func() {
		if conn != nil {
			conn.Close()
		}
		if db != nil {
			_ = db.Close()
		}
	}()

	if needDB {
		if db, err = store.Open(ctx, cfg.Database); err != nil {
			return err
		}
		applied, err := store.Migrate(ctx, db, opts.Migrations)
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			log.Info("migrations applied", "count", len(applied), "versions", applied)
		}
	}
	if cfg.NATSURL == "" {
		conn, err = queue.Start(ctx, cfg)
	} else {
		conn, err = queue.Connect(ctx, cfg)
	}
	if err != nil {
		return err
	}
	objs, err := queue.Ensure(ctx, conn.JS, queue.LimitsFromConfig(cfg))
	if err != nil {
		return err
	}

	ready := make(chan struct{})
	mux := newMux(ready)
	addr := cfg.HTTPAddr
	if cfg.Mode == "ingest" {
		addr = cfg.IngestAddr
	}
	srv, err := listen(addr, mux, log)
	if err != nil {
		return err
	}

	limits := opts.LimitsSource
	if limits == nil {
		limits = selfhost.LimitsFromConfig(cfg)
	}
	deps := ext.Deps{
		Logger:   log,
		Settings: ext.Settings{HTTPAddr: cfg.HTTPAddr, DataDir: cfg.DataDir, Mode: cfg.Mode},
	}
	if db != nil {
		deps.DB, deps.Dialect = db.SQL, string(db.Dialect)
	}
	var kw *keysync.Writer
	if writesKeys {
		kw = &keysync.Writer{DB: db, KV: objs.Keys, Limits: limits, Log: log}
		deps.Keys = kw // only here, so Keys is a nil interface elsewhere
	}

	// Components run on a context that outlives ctx until the HTTP server is down.
	cctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	g := newGroup()
	defer func() {
		srv.stop(log)
		cancel()
		g.wait()
	}()

	if selfHost && writesKeys {
		if _, err := selfhost.Bootstrap(ctx, db, kw, log, time.Now(), cfg.AdminEmail); err != nil {
			return err
		}
	}
	var apiHandler http.Handler
	var svc *service.Service // shared by the API and MCP handlers
	if writesKeys {
		svc = &service.Service{DB: db, Keys: kw, Queue: objs, Log: log, Version: Version, Mode: cfg.Mode}
		deps.Sessions = api.NewSessionStore(svc)
		auth := opts.Authenticator
		if auth == nil {
			var tenants ext.TenantResolver = selfhost.TenantResolver{DB: db}
			if opts.TenantResolver != nil {
				tenants = opts.TenantResolver
			}
			auth = &selfhost.Auth{Svc: svc, Sessions: deps.Sessions, Tenants: tenants}
		}
		apiHandler = api.New(api.Config{Svc: svc, Auth: auth, Deps: deps, Log: log})
	}
	if writesKeys {
		g.start("keysync", func() error { kw.Run(cctx, cfg.KeyCacheResyncInterval); return nil })
	}
	if runsIngest {
		mountIngest(cctx, g, mux, cfg, conn, objs, ready, log)
	}
	if runsProcessor {
		p := processor.New(db, conn.JS, objs.Consumer, processor.ConfigFrom(cfg), log)
		g.start("processor", func() error { return p.Run(cctx) })
	}
	if cfg.Mode == "api" || cfg.Mode == "all" {
		mux.Handle("/api/", apiHandler)
		// Mounted at both patterns so the handler's own path check answers every
		// /mcp/... sub-path, for any method, with a JSON 404 (the UI fallthrough
		// would answer POST with 405).
		mcpHandler := mcp.New(svc, log)
		mux.Handle("/mcp", mcpHandler)
		mux.Handle("/mcp/", mcpHandler)
		// The admin UI answers everything else; more specific patterns (/healthz,
		// /readyz, /api/, /v1/feedback, extension routes) still win.
		if sub, err := fs.Sub(web.Dist, "dist"); err == nil {
			mux.Handle("/", ui.Handler(sub))
		}
		if opts.MountRoutes != nil {
			opts.MountRoutes(mux, deps)
		}
	}
	for _, hook := range opts.OnStart {
		if err := hook(ctx, deps); err != nil {
			return fmt.Errorf("on start: %w", err)
		}
	}
	if !runsIngest {
		close(ready)
	}

	srv.serve(g)
	log.Info("feedbaxter started", "mode", cfg.Mode, "addr", srv.ln.Addr().String(), "version", Version)
	select {
	case <-ctx.Done():
		log.Info("shutting down")
		return nil
	case err := <-g.failed:
		return err
	}
}

// mountIngest wires the ingest stack. ready is closed once the key cache has
// replayed and the counters are loaded.
func mountIngest(ctx context.Context, g *group, mux *http.ServeMux, cfg config.Config,
	conn *queue.Conn, objs *queue.Objects, ready chan struct{}, log *slog.Logger) {
	cache := ingest.NewKeyCache(log)
	limiter := ingest.NewLimiter(cfg.RateLimitPerKey, cfg.RateLimitWindow)
	counters := ingest.NewCounters(conn.JS, objs.Counters, cfg.CounterFlushInterval, log)
	mux.Handle("/v1/feedback", ingest.NewHandler(ingest.Deps{
		Cache: cache, Limiter: limiter, Counters: counters, JS: conn.JS,
		Config: ingest.ConfigFrom(cfg), Ready: ready, Log: log,
	}))
	g.start("key cache", func() error { return cache.Run(ctx, objs.Keys) })
	g.start("limiter", func() error { limiter.Run(ctx); return nil })
	g.start("counters", func() error {
		select {
		case <-cache.Ready():
		case <-ctx.Done():
			return nil
		}
		if err := counters.Load(ctx, cache.Tenants(), time.Now()); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		close(ready)
		counters.Run(ctx)
		return nil
	})
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

// ResetAdmin gives an admin a new temporary password (see selfhost.ResetAdmin)
// and returns the admin's email and the password. It opens the database from the
// configuration and applies migrations first; the server need not be running.
func ResetAdmin(ctx context.Context, opts Options, email string) (string, string, error) {
	cfg, err := config.Load(nil)
	if err != nil {
		return "", "", err
	}
	db, err := store.Open(ctx, cfg.Database)
	if err != nil {
		return "", "", err
	}
	defer db.Close()
	if _, err := store.Migrate(ctx, db, opts.Migrations); err != nil {
		return "", "", err
	}
	var tenantID string
	if opts.TenantResolver != nil {
		tn, err := opts.TenantResolver.Default(ctx)
		if err != nil {
			return "", "", err
		}
		tenantID = tn.ID
	} else {
		tn, err := store.FirstTenant(ctx, db.SQL, db.Dialect)
		if errors.Is(err, store.ErrNotFound) {
			return "", "", errors.New("reset-admin: there is no organisation yet; start the server once first")
		}
		if err != nil {
			return "", "", err
		}
		tenantID = tn.ID
	}
	return selfhost.ResetAdmin(ctx, db, tenantID, email, time.Now())
}
