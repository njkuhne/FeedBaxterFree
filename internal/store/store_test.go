package store_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/store"
)

func TestOpenSQLitePragmas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "fb.db")
	db, err := store.Open(context.Background(), config.Database{Driver: "sqlite", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Dialect != store.SQLite {
		t.Errorf("dialect = %q", db.Dialect)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("parent directory not created: %v", err)
	}
	want := map[string]string{
		"journal_mode": "wal",
		"foreign_keys": "1",
		"busy_timeout": "5000",
		"synchronous":  "1",
	}
	for pragma, w := range want {
		var got string
		if err := db.SQL.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatalf("PRAGMA %s: %v", pragma, err)
		}
		if got != w {
			t.Errorf("PRAGMA %s = %q, want %q", pragma, got, w)
		}
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	_, err := store.Open(context.Background(), config.Database{Driver: "mysql", URL: "mysql://secret@host/db"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks URL: %v", err)
	}
}

func TestOpenPostgresErrorRedacted(t *testing.T) {
	const pw = "s3cretpw"
	_, err := store.Open(context.Background(), config.Database{
		Driver: "postgres",
		URL:    "postgres://fbuser:" + pw + "@127.0.0.1:1/nodb?sslmode=disable&connect_timeout=1",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), pw) {
		t.Errorf("error leaks password: %v", err)
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("error does not name the driver: %v", err)
	}
	t.Log(err)
}
