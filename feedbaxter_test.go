package feedbaxter

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxter/ext"
)

// isolate points the config at a temp SQLite file and an empty working directory.
func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "fb.db"))
}

func TestRunNotImplemented(t *testing.T) {
	isolate(t)
	err := Run(context.Background(), Options{Mode: "all"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Run error = %v, want ErrNotImplemented", err)
	}
}

func TestRunRejectsBadMode(t *testing.T) {
	isolate(t)
	err := Run(context.Background(), Options{Mode: "nope"})
	if err == nil || errors.Is(err, ErrNotImplemented) || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Run error = %v, want bad-mode error naming nope", err)
	}
}

func TestMigrateSQLite(t *testing.T) {
	isolate(t)
	got, err := Migrate(context.Background(), Options{})
	if err != nil || !reflect.DeepEqual(got, []string{"0001_core"}) {
		t.Fatalf("first Migrate = %v, %v; want [0001_core]", got, err)
	}
	got, err = Migrate(context.Background(), Options{})
	if err != nil || len(got) != 0 {
		t.Fatalf("second Migrate = %v, %v; want none", got, err)
	}
}

func TestMigrateWithExtension(t *testing.T) {
	isolate(t)
	m := ext.Migration{
		Version:  "ext_0001_demo",
		SQLite:   "CREATE TABLE ext_demo (id INTEGER PRIMARY KEY)",
		Postgres: "CREATE TABLE ext_demo (id BIGINT PRIMARY KEY)",
	}
	got, err := Migrate(context.Background(), Options{Migrations: []ext.Migration{m}})
	if err != nil || !reflect.DeepEqual(got, []string{"0001_core", "ext_0001_demo"}) {
		t.Fatalf("Migrate = %v, %v; want [0001_core ext_0001_demo]", got, err)
	}
}
