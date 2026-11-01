package store

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestMigrationFilenames(t *testing.T) {
	ok := fstest.MapFS{
		"m/0001_core.sql":           {Data: []byte("x")},
		"m/0002_split.sqlite.sql":   {Data: []byte("a")},
		"m/0002_split.postgres.sql": {Data: []byte("b")},
	}
	got, err := loadMigrations(ok, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].version != "0001_core" || got[1].version != "0002_split" ||
		got[1].sqlite != "a" || got[1].postgres != "b" {
		t.Fatalf("unexpected result: %+v", got)
	}

	bad := map[string]fstest.MapFS{
		"malformed":      {"m/core.sql": {Data: []byte("x")}},
		"bad suffix":     {"m/0001_core.txt": {Data: []byte("x")}},
		"uppercase":      {"m/0001_Core.sql": {Data: []byte("x")}},
		"unpaired":       {"m/0001_a.sqlite.sql": {Data: []byte("x")}},
		"unpaired pg":    {"m/0001_a.postgres.sql": {Data: []byte("x")}},
		"shared and one": {"m/0001_a.sql": {Data: []byte("x")}, "m/0001_a.sqlite.sql": {Data: []byte("x")}},
	}
	for name, fsys := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(fsys, "m"); err == nil {
				t.Error("expected error")
			} else if !strings.Contains(err.Error(), "migration") {
				t.Errorf("unhelpful error: %v", err)
			}
		})
	}
}
