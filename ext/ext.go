// Package ext is the exported extension surface of FeedBaxter. The private SaaS
// repo imports this module and fills feedbaxter.Options with implementations of
// the types below. It imports only the standard library.
package ext

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
)

// Tenant is one customer organisation.
type Tenant struct {
	// ID is the tenant identifier (prefix "tn_").
	ID string
	// Name is the display name.
	Name string
	// BillingMonthStart is the day of month (1..28) on which the billing period starts.
	BillingMonthStart int
}

// TenantResolver maps an authenticated principal or an ingest key to a tenant.
// Self-host: always the single tenant. SaaS: by row.
type TenantResolver interface {
	// ByID returns the tenant with the given ID.
	ByID(ctx context.Context, id string) (Tenant, error)
	// Default returns the tenant used when there is exactly one (self-host bootstrap, CLI).
	Default(ctx context.Context) (Tenant, error)
}

// Authenticator produces the session principal for the api mode.
// Self-host: password and sessions. SaaS: email flows (private).
type Authenticator interface {
	// Mount registers login, logout, password change and invite routes on the
	// mux that serves /api/. Every request there has already passed the CSRF
	// check and carries the resolved principal (if any) via PrincipalFrom.
	Mount(mux *http.ServeMux, deps Deps)
	// Principal resolves the session cookie of r to a Principal.
	Principal(r *http.Request) (Principal, bool)
}

// Role is a user's permission level.
type Role string

const (
	// RoleAdmin may do everything, including deleting feedback and managing channels, keys and people.
	RoleAdmin Role = "admin"
	// RoleReviewer may view, set status, tag, export and use MCP.
	RoleReviewer Role = "reviewer"
)

// Principal is the authenticated caller of an API or MCP request.
type Principal struct {
	// UserID is the user's ID.
	UserID string
	// TenantID is the tenant the user belongs to.
	TenantID string
	// Role is the user's role.
	Role Role
	// Via is how the caller authenticated: "ui" or "mcp".
	Via string
	// MustChangePassword is set while the user still has a temporary password.
	// The api then refuses every route except login, logout, password change and /api/me.
	MustChangePassword bool
}

type principalKey struct{}

// WithPrincipal returns a context that carries p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal stored by WithPrincipal.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Settings are the process settings an extension may read.
type Settings struct {
	// HTTPAddr is the listen address of the UI, API and MCP listener.
	HTTPAddr string
	// DataDir is the directory for the SQLite file and the embedded NATS store.
	DataDir string
	// Mode is ingest, worker, api or all.
	Mode string
}

// SessionStore issues and resolves fb_session cookies. One implementation
// (internal/api) owns the cookie rules for every Authenticator: a random
// 32-byte token of which only the sha256 is stored, HttpOnly, SameSite=Lax,
// Secure when the request is TLS or X-Forwarded-Proto is https, Path=/, and a
// 30-day sliding expiry.
type SessionStore interface {
	// Start creates a session for the user and sets the cookie on w.
	Start(w http.ResponseWriter, r *http.Request, userID, tenantID string) error
	// Principal resolves the cookie of r; Via is "ui".
	Principal(r *http.Request) (Principal, bool)
	// End deletes the session of r, if any, and clears the cookie.
	End(w http.ResponseWriter, r *http.Request) error
}

// LimitsSource decides daily and monthly limits for a tenant.
// Self-host: config. SaaS: plans.
type LimitsSource interface {
	// Limits returns the current limits for tenantID.
	Limits(ctx context.Context, tenantID string) (Limits, error)
}

// Limits are the per-tenant submission caps. Zero means unlimited.
type Limits struct {
	// Daily is the maximum submissions per UTC day.
	Daily int64
	// Monthly is the maximum submissions per billing month.
	Monthly int64
}

// Deps is what extensions receive. M2 may add more. Exported types only.
type Deps struct {
	// DB is the database handle.
	DB *sql.DB
	// Dialect is "sqlite" or "postgres".
	Dialect string
	// Logger is the process logger.
	Logger *slog.Logger
	// Keys rewrites ingest key cache entries after key, channel, tenant or limit
	// changes. Nil in the ingest and worker modes, which do not write the cache.
	Keys KeyWriter
	// Settings are the process settings, filled from the configuration.
	Settings Settings
	// Sessions issues and resolves session cookies. Nil outside the api and all modes.
	Sessions SessionStore
}

// Migration is an extension migration, applied after core migrations in its own
// version namespace. Version must start with "ext_" (e.g. "ext_0001_plans").
// Both SQL bodies are required.
type Migration struct {
	// Version is the unique migration name, starting with "ext_".
	Version string
	// SQLite is the SQL body for SQLite.
	SQLite string
	// Postgres is the SQL body for Postgres.
	Postgres string
}

// KeyWriter rewrites ingest key cache entries from the database. The api mode calls it after every
// key, channel, tenant or limit change; a periodic reconcile repairs anything missed.
type KeyWriter interface {
	// SyncKey upserts the entry of one key (revoked keys included).
	SyncKey(ctx context.Context, keyID string) error
	// SyncTenant upserts the tenant's keys and deletes its orphaned entries.
	SyncTenant(ctx context.Context, tenantID string) error
}
