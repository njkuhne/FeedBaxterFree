// Package storetest runs a test body against every available dialect.
package storetest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// ForEachDialect runs fn as subtest "sqlite" on a migrated SQLite DB in
// t.TempDir(), and as subtest "postgres" on a migrated, isolated Postgres
// schema when TEST_DATABASE_URL is set (else that subtest is skipped).
func ForEachDialect(t *testing.T, fn func(t *testing.T, db *store.DB)) {
	t.Helper()
	forEach(t, true, fn)
}

// ForEachDialectEmpty is ForEachDialect with no migrations applied.
func ForEachDialectEmpty(t *testing.T, fn func(t *testing.T, db *store.DB)) {
	t.Helper()
	forEach(t, false, fn)
}

func forEach(t *testing.T, migrate bool, fn func(t *testing.T, db *store.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db := openSQLite(t)
		prepare(t, db, migrate)
		fn(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		base := os.Getenv("TEST_DATABASE_URL")
		if base == "" {
			t.Skip("TEST_DATABASE_URL not set; skipping postgres")
		}
		db := openPostgres(t, base)
		prepare(t, db, migrate)
		fn(t, db)
	})
}

func prepare(t *testing.T, db *store.DB, migrate bool) {
	t.Helper()
	if !migrate {
		return
	}
	if _, err := store.Migrate(context.Background(), db, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func openSQLite(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), config.Database{
		Driver: "sqlite",
		Path:   filepath.Join(t.TempDir(), "fb.db"),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func openPostgres(t *testing.T, base string) *store.DB {
	t.Helper()
	ctx := context.Background()

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("open postgres admin: %v", err)
	}
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(rnd[:])
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close()
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(cctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		_ = admin.Close()
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal("TEST_DATABASE_URL is not a valid URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	db, err := store.Open(ctx, config.Database{Driver: "postgres", URL: u.String()})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	// Registered after the schema cleanup, so it runs first (LIFO): close
	// the pool before dropping the schema.
	t.Cleanup(func() { _ = db.Close() })
	return db
}
