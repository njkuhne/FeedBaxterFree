package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Session is one row of sessions. ID is the lowercase hex sha256 of the
// cookie token; the token itself is never stored.
type Session struct {
	ID, TenantID, UserID string
	ExpiresAt, CreatedAt int64
}

// CreateSession inserts s.
func CreateSession(ctx context.Context, q Querier, d Dialect, s Session) error {
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO sessions (id, tenant_id, user_id, expires_at, created_at) VALUES (?, ?, ?, ?, ?)"),
		s.ID, s.TenantID, s.UserID, s.ExpiresAt, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: create session: %w", err)
	}
	return nil
}

// GetSessionUser returns the session and its user, or ErrNotFound. It does
// not check expiry; the service does, so it can delete the row.
func GetSessionUser(ctx context.Context, q Querier, d Dialect, id string) (Session, User, error) {
	var (
		s    Session
		u    User
		must int
	)
	err := q.QueryRowContext(ctx, d.Rebind(
		"SELECT s.id, s.tenant_id, s.user_id, s.expires_at, s.created_at, "+userColsU+
			" FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id = ?"), id).Scan(
		&s.ID, &s.TenantID, &s.UserID, &s.ExpiresAt, &s.CreatedAt,
		&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &must, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, fmt.Errorf("store: get session: %w", ErrNotFound)
	}
	if err != nil {
		return Session{}, User{}, fmt.Errorf("store: get session: %w", err)
	}
	u.MustChangePassword = must != 0
	return s, u, nil
}

// SetSessionExpiry updates the session's expiry. A missing row is not an error.
func SetSessionExpiry(ctx context.Context, q Querier, d Dialect, id string, expiresAt int64) error {
	if _, err := q.ExecContext(ctx, d.Rebind("UPDATE sessions SET expires_at = ? WHERE id = ?"), expiresAt, id); err != nil {
		return fmt.Errorf("store: set session expiry: %w", err)
	}
	return nil
}

// DeleteSession removes the session; no error when absent.
func DeleteSession(ctx context.Context, q Querier, d Dialect, id string) error {
	if _, err := q.ExecContext(ctx, d.Rebind("DELETE FROM sessions WHERE id = ?"), id); err != nil {
		return fmt.Errorf("store: delete session: %w", err)
	}
	return nil
}

func execCount(ctx context.Context, q Querier, d Dialect, op, query string, args ...any) (int64, error) {
	res, err := q.ExecContext(ctx, d.Rebind(query), args...)
	if err != nil {
		return 0, fmt.Errorf("store: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: %s: %w", op, err)
	}
	return n, nil
}

// DeleteUserSessions deletes the user's sessions except exceptID ("" = all)
// and returns how many were deleted.
func DeleteUserSessions(ctx context.Context, q Querier, d Dialect, tenantID, userID, exceptID string) (int64, error) {
	if exceptID == "" {
		return execCount(ctx, q, d, "delete user sessions",
			"DELETE FROM sessions WHERE tenant_id = ? AND user_id = ?", tenantID, userID)
	}
	return execCount(ctx, q, d, "delete user sessions",
		"DELETE FROM sessions WHERE tenant_id = ? AND user_id = ? AND id <> ?", tenantID, userID, exceptID)
}

// DeleteExpiredSessions deletes sessions with expires_at <= now.
func DeleteExpiredSessions(ctx context.Context, q Querier, d Dialect, now int64) (int64, error) {
	return execCount(ctx, q, d, "delete expired sessions", "DELETE FROM sessions WHERE expires_at <= ?", now)
}
