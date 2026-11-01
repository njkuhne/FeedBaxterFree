package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// User is one row of users.
type User struct {
	ID, TenantID, Email, PasswordHash, Role string
	MustChangePassword                      bool
	CreatedAt, UpdatedAt                    int64
}

const userCols = "id, tenant_id, email, password_hash, role, must_change_password, created_at, updated_at"

// userColsU is userCols qualified with the alias u.
const userColsU = "u.id, u.tenant_id, u.email, u.password_hash, u.role, u.must_change_password, u.created_at, u.updated_at"

type scanner interface{ Scan(...any) error }

func scanUser(s scanner) (User, error) {
	var (
		u    User
		must int
	)
	err := s.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &must, &u.CreatedAt, &u.UpdatedAt)
	u.MustChangePassword = must != 0
	return u, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// CreateUser inserts u, assigning an ID when empty. A duplicate
// (tenant_id, email) returns ErrConflict.
func CreateUser(ctx context.Context, q Querier, d Dialect, u User) (User, error) {
	if u.ID == "" {
		u.ID = NewID(PrefixUser)
	}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO users ("+userCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)"),
		u.ID, u.TenantID, u.Email, u.PasswordHash, u.Role, boolInt(u.MustChangePassword), u.CreatedAt, u.UpdatedAt)
	if isUniqueViolation(err) {
		return User{}, fmt.Errorf("store: create user: %w", ErrConflict)
	}
	if err != nil {
		return User{}, fmt.Errorf("store: create user: %w", err)
	}
	return u, nil
}

func getUser(ctx context.Context, q Querier, d Dialect, op, where string, args ...any) (User, error) {
	u, err := scanUser(q.QueryRowContext(ctx, d.Rebind("SELECT "+userCols+" FROM users WHERE "+where), args...))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, fmt.Errorf("store: %s: %w", op, ErrNotFound)
	}
	if err != nil {
		return User{}, fmt.Errorf("store: %s: %w", op, err)
	}
	return u, nil
}

// GetUser returns ErrNotFound when the user is not in the tenant.
func GetUser(ctx context.Context, q Querier, d Dialect, tenantID, id string) (User, error) {
	return getUser(ctx, q, d, "get user", "tenant_id = ? AND id = ?", tenantID, id)
}

// GetUserByEmail matches email exactly; the service lower-cases it.
func GetUserByEmail(ctx context.Context, q Querier, d Dialect, tenantID, email string) (User, error) {
	return getUser(ctx, q, d, "get user by email", "tenant_id = ? AND email = ?", tenantID, email)
}

// FirstAdmin returns the oldest admin, or ErrNotFound.
func FirstAdmin(ctx context.Context, q Querier, d Dialect, tenantID string) (User, error) {
	return getUser(ctx, q, d, "first admin", "tenant_id = ? AND role = 'admin' ORDER BY created_at, id LIMIT 1", tenantID)
}

// ListUsers returns the tenant's users ordered by created_at, id.
func ListUsers(ctx context.Context, q Querier, d Dialect, tenantID string) ([]User, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT "+userCols+" FROM users WHERE tenant_id = ? ORDER BY created_at, id"), tenantID)
	if err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list users: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list users: %w", err)
	}
	return out, nil
}

func countWhere(ctx context.Context, q Querier, d Dialect, op, query string, args ...any) (int, error) {
	var n int
	if err := q.QueryRowContext(ctx, d.Rebind(query), args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: %s: %w", op, err)
	}
	return n, nil
}

// CountUsers returns the number of users in the tenant.
func CountUsers(ctx context.Context, q Querier, d Dialect, tenantID string) (int, error) {
	return countWhere(ctx, q, d, "count users", "SELECT COUNT(*) FROM users WHERE tenant_id = ?", tenantID)
}

// CountAdmins returns the number of admins in the tenant.
func CountAdmins(ctx context.Context, q Querier, d Dialect, tenantID string) (int, error) {
	return countWhere(ctx, q, d, "count admins", "SELECT COUNT(*) FROM users WHERE tenant_id = ? AND role = 'admin'", tenantID)
}

// LockAdmins serialises last-admin checks. Postgres locks the tenant's admin
// rows (call inside a transaction); SQLite does nothing because its
// transactions are BEGIN IMMEDIATE, so writers are already serialised.
func LockAdmins(ctx context.Context, q Querier, d Dialect, tenantID string) error {
	if d != Postgres {
		return nil
	}
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT id FROM users WHERE tenant_id = ? AND role = 'admin' ORDER BY id FOR UPDATE"), tenantID)
	if err != nil {
		return fmt.Errorf("store: lock admins: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: lock admins: %w", err)
	}
	return nil
}

func execOne(ctx context.Context, q Querier, d Dialect, op, query string, args ...any) error {
	res, err := q.ExecContext(ctx, d.Rebind(query), args...)
	if err != nil {
		return fmt.Errorf("store: %s: %w", op, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s: %w", op, err)
	}
	if n == 0 {
		return fmt.Errorf("store: %s: %w", op, ErrNotFound)
	}
	return nil
}

// SetUserRole returns ErrNotFound when the user is not in the tenant.
func SetUserRole(ctx context.Context, q Querier, d Dialect, tenantID, id, role string, now int64) error {
	return execOne(ctx, q, d, "set user role",
		"UPDATE users SET role = ?, updated_at = ? WHERE tenant_id = ? AND id = ?", role, now, tenantID, id)
}

// SetUserPassword returns ErrNotFound when the user is not in the tenant.
func SetUserPassword(ctx context.Context, q Querier, d Dialect, tenantID, id, hash string, mustChange bool, now int64) error {
	return execOne(ctx, q, d, "set user password",
		"UPDATE users SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE tenant_id = ? AND id = ?",
		hash, boolInt(mustChange), now, tenantID, id)
}

// DeleteUser removes the user; its sessions and tokens go by FK cascade.
func DeleteUser(ctx context.Context, q Querier, d Dialect, tenantID, id string) error {
	return execOne(ctx, q, d, "delete user", "DELETE FROM users WHERE tenant_id = ? AND id = ?", tenantID, id)
}
