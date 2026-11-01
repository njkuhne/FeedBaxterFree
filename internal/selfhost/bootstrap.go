package selfhost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// DefaultAdminEmail is the admin created when none is configured.
const DefaultAdminEmail = "admin@localhost"

func normaliseEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// newAdmin inserts an admin with a fresh temporary password that must be
// changed at first login, and returns the user and the plaintext.
func newAdmin(ctx context.Context, q store.Querier, d store.Dialect, tenantID, email string, nowMs int64) (store.User, string, error) {
	pw := service.GenerateTempPassword()
	hash, err := service.HashPassword(pw)
	if err != nil {
		return store.User{}, "", err
	}
	u, err := store.CreateUser(ctx, q, d, store.User{
		TenantID: tenantID, Email: email, PasswordHash: hash, Role: string(ext.RoleAdmin),
		MustChangePassword: true, CreatedAt: nowMs, UpdatedAt: nowMs,
	})
	return u, pw, err
}

// Bootstrap makes a database usable on first boot. When there is no tenant it
// creates the tenant, the default channel and one ingest key (logged once).
// Independently, when the tenant has no users (also an M1 database upgraded in
// place) it creates the admin with a temporary password that is logged once,
// after the commit. created reports whether anything was created.
func Bootstrap(ctx context.Context, db *store.DB, keys ext.KeyWriter, log *slog.Logger, now time.Time, adminEmail string) (created bool, err error) {
	adminEmail = normaliseEmail(adminEmail)
	if adminEmail == "" {
		adminEmail = DefaultAdminEmail
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("selfhost: bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ms := now.UnixMilli()
	var (
		tenantID string
		key      string
		keyRow   store.IngestKey
		adminPW  string
		madeKey  bool
	)
	n, err := store.CountTenants(ctx, tx, db.Dialect)
	if err != nil {
		return false, err
	}
	if n == 0 {
		tn, err := store.CreateTenant(ctx, tx, db.Dialect, TenantName, billingMonthStart, ms)
		if err != nil {
			return false, err
		}
		ch, err := store.CreateChannel(ctx, tx, db.Dialect, tn.ID, DefaultChannel, ms)
		if err != nil {
			return false, err
		}
		key = store.GenerateIngestKey()
		if keyRow, err = store.CreateIngestKey(ctx, tx, db.Dialect, tn.ID, ch.ID, key, ms); err != nil {
			return false, err
		}
		tenantID, madeKey = tn.ID, true
	} else {
		tn, err := store.FirstTenant(ctx, tx, db.Dialect)
		if err != nil {
			return false, err
		}
		tenantID = tn.ID
	}
	users, err := store.CountUsers(ctx, tx, db.Dialect, tenantID)
	if err != nil {
		return false, err
	}
	madeAdmin := users == 0
	if madeAdmin {
		if _, adminPW, err = newAdmin(ctx, tx, db.Dialect, tenantID, adminEmail, ms); err != nil {
			return false, err
		}
	}
	if !madeKey && !madeAdmin {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("selfhost: bootstrap: commit: %w", err)
	}

	if madeKey {
		// The reconcile repairs a failed sync, so this is not fatal.
		if err := keys.SyncKey(ctx, keyRow.ID); err != nil {
			log.Warn("selfhost: key cache sync failed; the periodic reconcile will retry", "err", err)
		}
		log.Info("ingest key for channel " + DefaultChannel + ": " + key)
	}
	if madeAdmin {
		log.Info("admin login " + adminEmail + " temporary password: " + adminPW + " (you must change it at first login)")
	}
	return true, nil
}

// ResetAdmin gives an admin a new temporary password, forces a change at next
// login and ends their sessions. With an email it must name an admin; without,
// it takes the first admin. A tenant with no users at all gets a new admin
// (email, or admin@localhost). It returns the email and the plaintext password.
func ResetAdmin(ctx context.Context, db *store.DB, tenantID, email string, now time.Time) (gotEmail, pw string, err error) {
	email = normaliseEmail(email)
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("selfhost: reset admin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	ms := now.UnixMilli()

	users, err := store.CountUsers(ctx, tx, db.Dialect, tenantID)
	if err != nil {
		return "", "", err
	}
	if users == 0 {
		if email == "" {
			email = DefaultAdminEmail
		}
		u, pw, err := newAdmin(ctx, tx, db.Dialect, tenantID, email, ms)
		if err != nil {
			return "", "", err
		}
		if err := tx.Commit(); err != nil {
			return "", "", fmt.Errorf("selfhost: reset admin: commit: %w", err)
		}
		return u.Email, pw, nil
	}

	var u store.User
	if email != "" {
		u, err = store.GetUserByEmail(ctx, tx, db.Dialect, tenantID, email)
		if errors.Is(err, store.ErrNotFound) {
			return "", "", fmt.Errorf("no user with email %s", email)
		}
	} else {
		u, err = store.FirstAdmin(ctx, tx, db.Dialect, tenantID)
		if errors.Is(err, store.ErrNotFound) {
			return "", "", errors.New("there is no admin; pass --email to name a user")
		}
	}
	if err != nil {
		return "", "", err
	}
	if u.Role != string(ext.RoleAdmin) {
		return "", "", errors.New("user is not an admin")
	}
	pw = service.GenerateTempPassword()
	hash, err := service.HashPassword(pw)
	if err != nil {
		return "", "", err
	}
	if err := store.SetUserPassword(ctx, tx, db.Dialect, tenantID, u.ID, hash, true, ms); err != nil {
		return "", "", err
	}
	if _, err := store.DeleteUserSessions(ctx, tx, db.Dialect, tenantID, u.ID, ""); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("selfhost: reset admin: commit: %w", err)
	}
	return u.Email, pw, nil
}
