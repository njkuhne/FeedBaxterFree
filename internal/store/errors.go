package store

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is returned when a row looked up by key does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned when a write violates a unique constraint.
var ErrConflict = errors.New("store: conflict")

// isUniqueViolation reports whether err is a unique-constraint violation on
// either Postgres (SQLSTATE 23505) or SQLite.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
