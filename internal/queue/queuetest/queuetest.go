// Package queuetest starts an embedded NATS for tests.
package queuetest

import (
	"context"
	"testing"

	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/queue"
)

const (
	defMaxMsgs  = 100000
	defMaxBytes = 256 << 20
)

// Embedded starts an embedded server with its store in t.TempDir(), runs
// Ensure with lim (zero fields get defaults) and closes it in t.Cleanup.
func Embedded(t testing.TB, lim queue.Limits) (*queue.Conn, *queue.Objects) {
	t.Helper()
	if lim.MaxMsgs == 0 {
		lim.MaxMsgs = defMaxMsgs
	}
	if lim.MaxBytes == 0 {
		lim.MaxBytes = defMaxBytes
	}
	ctx := context.Background()
	c, err := queue.Start(ctx, config.Config{DataDir: t.TempDir(), Mode: "all"})
	if err != nil {
		t.Fatalf("queue.Start: %v", err)
	}
	t.Cleanup(c.Close)
	o, err := queue.Ensure(ctx, c.JS, lim)
	if err != nil {
		t.Fatalf("queue.Ensure: %v", err)
	}
	return c, o
}
