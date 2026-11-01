package mcp

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/service"
)

func TestToolErrMapping(t *testing.T) {
	var buf bytes.Buffer
	h := &handler{log: slog.New(slog.NewTextHandler(&buf, nil))}
	kinds := []service.Kind{
		service.KindInvalid, service.KindUnauthenticated, service.KindForbidden, service.KindNotFound, service.KindConflict,
	}
	for _, k := range kinds {
		se := &service.Error{Kind: k, Msg: fmt.Sprintf("message for kind %d.", k)}
		if got := h.toolErr("x", se); got.Error() != se.Msg {
			t.Errorf("kind %d: %q", k, got)
		}
		if got := h.toolErr("x", fmt.Errorf("wrapped: %w", se)); got.Error() != se.Msg {
			t.Errorf("wrapped kind %d: %q", k, got)
		}
	}
	if buf.Len() != 0 {
		t.Errorf("service errors were logged: %s", buf.String())
	}
	got := h.toolErr("list_tags", errors.New("pq: secret driver detail"))
	if got.Error() != "Something went wrong." {
		t.Errorf("plain error = %q", got)
	}
	logs := buf.String()
	if strings.Count(logs, "\n") != 1 || !strings.Contains(logs, "level=ERROR") || !strings.Contains(logs, "tool=list_tags") {
		t.Errorf("log = %q", logs)
	}
}
