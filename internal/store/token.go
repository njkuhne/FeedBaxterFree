package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Token is one row of personal_access_tokens. Only the hash is stored.
type Token struct {
	ID, TenantID, UserID, Name, Prefix, Hash string
	LastUsedAt                               *int64
	CreatedAt                                int64
}

const tokenCols = "id, tenant_id, user_id, name, prefix, hash, last_used_at, created_at"

// CreateToken inserts t, assigning an ID when empty.
func CreateToken(ctx context.Context, q Querier, d Dialect, t Token) (Token, error) {
	if t.ID == "" {
		t.ID = NewID(PrefixToken)
	}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO personal_access_tokens ("+tokenCols+") VALUES (?, ?, ?, ?, ?, ?, ?, ?)"),
		t.ID, t.TenantID, t.UserID, t.Name, t.Prefix, t.Hash, t.LastUsedAt, t.CreatedAt)
	if err != nil {
		return Token{}, fmt.Errorf("store: create token: %w", err)
	}
	return t, nil
}

// ListTokens returns the user's tokens, newest first.
func ListTokens(ctx context.Context, q Querier, d Dialect, tenantID, userID string) ([]Token, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT "+tokenCols+" FROM personal_access_tokens WHERE tenant_id = ? AND user_id = ? ORDER BY created_at DESC, id DESC"),
		tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("store: list tokens: %w", err)
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.TenantID, &t.UserID, &t.Name, &t.Prefix, &t.Hash, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: list tokens: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list tokens: %w", err)
	}
	return out, nil
}

// DeleteToken returns ErrNotFound unless the row belongs to that user.
func DeleteToken(ctx context.Context, q Querier, d Dialect, tenantID, userID, id string) error {
	return execOne(ctx, q, d, "delete token",
		"DELETE FROM personal_access_tokens WHERE tenant_id = ? AND user_id = ? AND id = ?", tenantID, userID, id)
}

// GetTokenUser returns the token with the given hash and its owner, or
// ErrNotFound.
func GetTokenUser(ctx context.Context, q Querier, d Dialect, hash string) (Token, User, error) {
	var (
		t    Token
		u    User
		must int
	)
	err := q.QueryRowContext(ctx, d.Rebind(
		"SELECT t.id, t.tenant_id, t.user_id, t.name, t.prefix, t.hash, t.last_used_at, t.created_at, "+userColsU+
			" FROM personal_access_tokens t JOIN users u ON u.id = t.user_id WHERE t.hash = ?"), hash).Scan(
		&t.ID, &t.TenantID, &t.UserID, &t.Name, &t.Prefix, &t.Hash, &t.LastUsedAt, &t.CreatedAt,
		&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &must, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, User{}, fmt.Errorf("store: get token: %w", ErrNotFound)
	}
	if err != nil {
		return Token{}, User{}, fmt.Errorf("store: get token: %w", err)
	}
	u.MustChangePassword = must != 0
	return t, u, nil
}

// TouchToken sets last_used_at = now only when it is NULL or older than
// now-60000, so a busy token causes one write per minute.
func TouchToken(ctx context.Context, q Querier, d Dialect, id string, now int64) error {
	_, err := q.ExecContext(ctx, d.Rebind(
		"UPDATE personal_access_tokens SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)"),
		now, id, now-60000)
	if err != nil {
		return fmt.Errorf("store: touch token: %w", err)
	}
	return nil
}
