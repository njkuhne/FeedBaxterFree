package api

import (
	"strings"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/service"
)

var nameDay = time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC)

func TestExportFilenameAll(t *testing.T) {
	if got, want := exportFilename(service.Filter{}, "csv", nameDay), "feedbaxter-all-2026-11-01.csv"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// Channel, search and user filters never reach the name; the date is UTC.
	f := service.Filter{ChannelID: "ch_1", Q: "secret words", UserID: "u-42"}
	loc := time.FixedZone("late", -10*3600)
	if got, want := exportFilename(f, "json", time.Date(2026, 10, 31, 20, 0, 0, 0, loc)), "feedbaxter-all-2026-11-01.json"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExportFilenameStatusAndTag(t *testing.T) {
	f := service.Filter{Statuses: []string{"new"}, Tag: "bug"}
	if got, want := exportFilename(f, "json", nameDay), "feedbaxter-new-bug-2026-11-01.json"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	f = service.Filter{Statuses: []string{"new", "reviewed"}}
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-new-reviewed-2026-11-01.csv"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExportFilenameTagSanitised(t *testing.T) {
	f := service.Filter{Statuses: []string{"reviewed"}, Tag: "Safari Export!"}
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-reviewed-safari-export-2026-11-01.csv"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	f.Tag = "!!! ???"
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-reviewed-2026-11-01.csv"; got != want {
		t.Errorf("symbol-only tag: got %q, want %q", got, want)
	}
	f.Tag = strings.Repeat("a", 60)
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-reviewed-"+strings.Repeat("a", 40)+"-2026-11-01.csv"; got != want {
		t.Errorf("long tag: got %q, want %q", got, want)
	}
	// A cut that lands on a separator leaves no trailing dash.
	f.Tag = strings.Repeat("a", 39) + " bbb"
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-reviewed-"+strings.Repeat("a", 39)+"-2026-11-01.csv"; got != want {
		t.Errorf("cut at separator: got %q, want %q", got, want)
	}
	f.Tag = "  Ünï  --  ok\r\n\"x"
	if got, want := exportFilename(f, "csv", nameDay), "feedbaxter-reviewed-n-ok-x-2026-11-01.csv"; got != want {
		t.Errorf("odd chars: got %q, want %q", got, want)
	}
}
