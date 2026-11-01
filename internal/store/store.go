// Package store opens the database, applies migrations and holds the
// hand-written queries. Entity query functions take a Querier so they run in
// or out of a transaction.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
	_ "modernc.org/sqlite"             // registers the "sqlite" driver

	"github.com/njkuhne/feedbaxterfree/internal/config"
)

// DB is an open database and its dialect.
type DB struct {
	SQL     *sql.DB
	Dialect Dialect
}

// Querier is satisfied by *sql.DB, *sql.Tx and *sql.Conn.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open connects to the database described by cfg and verifies it with a ping.
// Errors name the driver and never include the database URL.
func Open(ctx context.Context, cfg config.Database) (*DB, error) {
	var (
		sqlDB   *sql.DB
		dialect Dialect
		err     error
	)
	switch cfg.Driver {
	case "sqlite":
		dialect = SQLite
		if cfg.Path == "" {
			return nil, fmt.Errorf("store: sqlite: empty database path")
		}
		if err = os.MkdirAll(filepath.Dir(cfg.Path), 0o750); err != nil {
			return nil, fmt.Errorf("store: sqlite: create data directory: %w", err)
		}
		dsn := "file:" + cfg.Path +
			"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)" +
			"&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"
		sqlDB, err = sql.Open("sqlite", dsn)
	case "postgres":
		dialect = Postgres
		sqlDB, err = sql.Open("pgx", cfg.URL)
	default:
		return nil, fmt.Errorf("store: unknown database driver %q", cfg.Driver)
	}
	if err != nil {
		return nil, fmt.Errorf("store: %s: open: %s", cfg.Driver, redact(err, cfg.URL))
	}
	if err = sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		if dialect == SQLite {
			return nil, fmt.Errorf("store: sqlite: ping: %w", err)
		}
		return nil, fmt.Errorf("store: postgres: ping: %s", redact(err, cfg.URL))
	}
	return &DB{SQL: sqlDB, Dialect: dialect}, nil
}

// Close closes the underlying pool.
func (db *DB) Close() error { return db.SQL.Close() }

// redact returns err's text with the full rawURL replaced by <DATABASE_URL>
// and any password found in rawURL replaced by <redacted>.
func redact(err error, rawURL string) string {
	msg := err.Error()
	if rawURL == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, rawURL, "<DATABASE_URL>")
	if u, perr := url.Parse(rawURL); perr == nil && u.User != nil {
		if pw, ok := u.User.Password(); ok && pw != "" {
			msg = strings.ReplaceAll(msg, pw, "<redacted>")
		}
	}
	return msg
}

// DatabaseSize returns the size of the database in bytes.
func DatabaseSize(ctx context.Context, db *DB) (int64, error) {
	query := "SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()"
	if db.Dialect == Postgres {
		query = "SELECT pg_database_size(current_database())"
	}
	var n int64
	if err := db.SQL.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: database size: %w", err)
	}
	return n, nil
}
