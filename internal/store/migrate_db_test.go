package store_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

var coreTables = []string{
	"schema_migrations", "tenants", "users", "sessions", "personal_access_tokens", "channels",
	"ingest_keys", "feedback", "tags", "feedback_tags", "status_changes",
}

func count(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var n int
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func TestMigrateFresh(t *testing.T) {
	storetest.ForEachDialectEmpty(t, func(t *testing.T, db *store.DB) {
		got, err := store.Migrate(context.Background(), db, nil)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"0001_core", "0002_triage", "0003_key_last_used"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("applied %v, want %v", got, want)
		}
		for _, tbl := range coreTables {
			count(t, db, tbl)
		}
	})
}

func TestMigrateIdempotent(t *testing.T) {
	storetest.ForEachDialectEmpty(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		if _, err := store.Migrate(ctx, db, nil); err != nil {
			t.Fatal(err)
		}
		got, err := store.Migrate(ctx, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 0 {
			t.Errorf("second call applied %v", got)
		}
		if n := count(t, db, "schema_migrations"); n != 3 {
			t.Errorf("schema_migrations rows = %d, want 3", n)
		}
	})
}

func TestMigrateExtension(t *testing.T) {
	demo := ext.Migration{
		Version:  "ext_0001_demo",
		SQLite:   "CREATE TABLE demo (id TEXT PRIMARY KEY);",
		Postgres: "CREATE TABLE demo (id TEXT PRIMARY KEY);",
	}
	storetest.ForEachDialectEmpty(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		got, err := store.Migrate(ctx, db, []ext.Migration{demo})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"0001_core", "0002_triage", "0003_key_last_used", "ext_0001_demo"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("applied %v, want %v", got, want)
		}
		count(t, db, "demo")
		got, err = store.Migrate(ctx, db, []ext.Migration{demo})
		if err != nil || len(got) != 0 {
			t.Fatalf("re-apply: %v, %v", got, err)
		}
	})

	storetest.ForEachDialectEmpty(t, func(t *testing.T, db *store.DB) {
		bad := []ext.Migration{demo, {Version: "0002_x", SQLite: "SELECT 1;", Postgres: "SELECT 1;"}}
		got, err := store.Migrate(context.Background(), db, bad)
		if err == nil {
			t.Fatal("expected error for bad version")
		}
		if len(got) != 0 {
			t.Errorf("applied %v despite error", got)
		}
		// schema_migrations is only created once validation passes, so
		// nothing at all may exist.
		if _, qerr := db.SQL.Exec("SELECT 1 FROM tenants"); qerr == nil {
			t.Error("core schema was applied despite rejected extension")
		}
	})
}

func TestMigration0003AddsLastUsedAt(t *testing.T) {
	storetest.ForEachDialectEmpty(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		if _, err := store.Migrate(ctx, db, nil); err != nil {
			t.Fatal(err)
		}
		tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, "acme", 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		ch, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, "main", 1000)
		if err != nil {
			t.Fatal(err)
		}
		k, err := store.CreateIngestKey(ctx, db.SQL, db.Dialect, tn.ID, ch.ID, store.GenerateIngestKey(), 2000)
		if err != nil {
			t.Fatal(err)
		}
		var last sql.NullInt64
		if err := db.SQL.QueryRow(db.Dialect.Rebind("SELECT last_used_at FROM ingest_keys WHERE id = ?"), k.ID).Scan(&last); err != nil {
			t.Fatalf("last_used_at column: %v", err)
		}
		if last.Valid {
			t.Errorf("new key last_used_at = %d, want NULL", last.Int64)
		}
		if k.LastUsedAt != nil {
			t.Errorf("created key LastUsedAt = %v", *k.LastUsedAt)
		}
	})
}
