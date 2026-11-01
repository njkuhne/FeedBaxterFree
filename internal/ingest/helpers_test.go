package ingest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const waitTimeout = 5 * time.Second

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// startCache runs a KeyCache against kv and waits until it is ready. The
// goroutine is stopped and joined at cleanup.
func startCache(t *testing.T, kv jetstream.KeyValue) *KeyCache {
	t.Helper()
	c := NewKeyCache(discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); _ = c.Run(ctx, kv) }()
	t.Cleanup(func() { cancel(); wg.Wait() })
	select {
	case <-c.Ready():
	case <-time.After(waitTimeout):
		t.Fatal("key cache not ready")
	}
	return c
}

func putEntry(t *testing.T, kv jetstream.KeyValue, plaintext string, e Entry) {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Put(context.Background(), HashKey(plaintext), b); err != nil {
		t.Fatal(err)
	}
}

// eventually polls cond until it holds or waitTimeout passes. Used only where
// a server-side watch delivery is what is under test.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
