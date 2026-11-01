package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
)

//go:embed migrations/*.sql
var coreFS embed.FS

// pgAdvisoryLockKey serialises concurrent Migrate calls on Postgres.
const pgAdvisoryLockKey int64 = 0x4642584d49475231 // "FBXMIGR1"

var (
	coreVersionRE = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+$`)
	extVersionRE  = regexp.MustCompile(`^ext_[0-9]{4}_[a-z0-9_]+$`)
)

type migration struct {
	version  string
	sqlite   string
	postgres string
}

// loadMigrations reads NNNN_name.sql (both dialects) or the pair
// NNNN_name.sqlite.sql + NNNN_name.postgres.sql from dir in fsys, sorted by version.
func loadMigrations(fsys fs.FS, dir string) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("store: read migrations: %w", err)
	}
	type parts struct{ both, sqlite, postgres *string }
	byVersion := map[string]*parts{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		base, ok := strings.CutSuffix(name, ".sql")
		if !ok {
			return nil, fmt.Errorf("store: migration %q: name must end in .sql", name)
		}
		kind := "both"
		if v, ok := strings.CutSuffix(base, ".sqlite"); ok {
			base, kind = v, "sqlite"
		} else if v, ok := strings.CutSuffix(base, ".postgres"); ok {
			base, kind = v, "postgres"
		}
		if !coreVersionRE.MatchString(base) {
			return nil, fmt.Errorf("store: migration %q: malformed name, want NNNN_name.sql", name)
		}
		body, err := fs.ReadFile(fsys, dir+"/"+name)
		if err != nil {
			return nil, fmt.Errorf("store: read migration %q: %w", name, err)
		}
		s := string(body)
		p := byVersion[base]
		if p == nil {
			p = &parts{}
			byVersion[base] = p
		}
		switch kind {
		case "both":
			p.both = &s
		case "sqlite":
			p.sqlite = &s
		case "postgres":
			p.postgres = &s
		}
	}
	var out []migration
	for v, p := range byVersion {
		switch {
		case p.both != nil && (p.sqlite != nil || p.postgres != nil):
			return nil, fmt.Errorf("store: migration %q: has both a shared file and dialect files", v)
		case p.both != nil:
			out = append(out, migration{v, *p.both, *p.both})
		case p.sqlite != nil && p.postgres != nil:
			out = append(out, migration{v, *p.sqlite, *p.postgres})
		default:
			return nil, fmt.Errorf("store: migration %q: needs both .sqlite.sql and .postgres.sql", v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func validateExt(extra []ext.Migration) ([]migration, error) {
	seen := map[string]bool{}
	var out []migration
	for _, m := range extra {
		if !extVersionRE.MatchString(m.Version) {
			return nil, fmt.Errorf("store: extension migration %q: version must match ext_NNNN_name", m.Version)
		}
		if seen[m.Version] {
			return nil, fmt.Errorf("store: extension migration %q: duplicate version", m.Version)
		}
		seen[m.Version] = true
		if strings.TrimSpace(m.SQLite) == "" || strings.TrimSpace(m.Postgres) == "" {
			return nil, fmt.Errorf("store: extension migration %q: both SQLite and Postgres bodies are required", m.Version)
		}
		out = append(out, migration{m.Version, m.SQLite, m.Postgres})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Migrate applies pending core migrations, then pending extension migrations
// (sorted by Version), each in its own transaction together with its
// schema_migrations row. It returns the versions applied by this call, in order.
func Migrate(ctx context.Context, db *DB, extra []ext.Migration) ([]string, error) {
	return migrate(ctx, db, coreFS, "migrations", extra)
}

func migrate(ctx context.Context, db *DB, fsys fs.FS, dir string, extra []ext.Migration) (applied []string, err error) {
	core, err := loadMigrations(fsys, dir)
	if err != nil {
		return nil, err
	}
	extMigs, err := validateExt(extra)
	if err != nil {
		return nil, err
	}
	all := append(core, extMigs...)

	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: migrate: acquire connection: %w", err)
	}
	defer conn.Close()

	if db.Dialect == Postgres {
		if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", pgAdvisoryLockKey); err != nil {
			return nil, fmt.Errorf("store: migrate: advisory lock: %w", err)
		}
		defer func() {
			// Use a fresh context so unlock still runs if ctx was cancelled.
			uctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, uerr := conn.ExecContext(uctx, "SELECT pg_advisory_unlock($1)", pgAdvisoryLockKey); uerr != nil {
				err = errors.Join(err, fmt.Errorf("store: migrate: advisory unlock: %w", uerr))
			}
		}()
	}

	if _, err := conn.ExecContext(ctx,
		"CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at BIGINT NOT NULL)"); err != nil {
		return nil, fmt.Errorf("store: migrate: create schema_migrations: %w", err)
	}
	done, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	for _, m := range all {
		if done[m.version] {
			continue
		}
		body := m.sqlite
		if db.Dialect == Postgres {
			body = m.postgres
		}
		body = strings.ReplaceAll(body, "{{json}}", db.Dialect.JSONType())
		if err := applyOne(ctx, db.Dialect, conn, m.version, body); err != nil {
			return applied, err
		}
		applied = append(applied, m.version)
	}
	return applied, nil
}

func appliedVersions(ctx context.Context, q Querier) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("store: migrate: read schema_migrations: %w", err)
	}
	defer rows.Close()
	done := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: migrate: scan schema_migrations: %w", err)
		}
		done[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: migrate: read schema_migrations: %w", err)
	}
	return done, nil
}

func applyOne(ctx context.Context, d Dialect, conn *sql.Conn, version, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: migrate %s: begin: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, body); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: migrate %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		d.Rebind("INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)"),
		version, time.Now().UnixMilli()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: migrate %s: record version: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: migrate %s: commit: %w", version, err)
	}
	return nil
}
