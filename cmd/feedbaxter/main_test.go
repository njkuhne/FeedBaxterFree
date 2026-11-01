package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func do(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestVersion(t *testing.T) {
	code, out, _ := do("version")
	if code != 0 || strings.TrimSpace(out) != "feedbaxter dev" {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, errs := do("bogus")
	if code != 2 || !strings.Contains(errs, "Usage:") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestNoCommand(t *testing.T) {
	code, _, errs := do()
	if code != 2 || !strings.Contains(errs, "Usage:") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestServeBadMode(t *testing.T) {
	isolate(t)
	code, _, errs := do("serve", "--mode=nope")
	if code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func TestResetAdminNotImplemented(t *testing.T) {
	code, _, errs := do("reset-admin")
	if code != 1 || !strings.Contains(errs, "M2") {
		t.Fatalf("code=%d stderr=%q", code, errs)
	}
}

func isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(dir, "fb.db"))
}

func TestMigrateCommand(t *testing.T) {
	isolate(t)
	code, out, errs := do("migrate")
	if code != 0 || !strings.Contains(out, "applied 0001_core") {
		t.Fatalf("first: code=%d out=%q stderr=%q", code, out, errs)
	}
	code, out, errs = do("migrate")
	if code != 0 || !strings.Contains(out, "schema is up to date") {
		t.Fatalf("second: code=%d out=%q stderr=%q", code, out, errs)
	}
}
