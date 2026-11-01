package ingest

import (
	"context"
	"testing"
	"time"
)

var t0 = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

func TestLimiterBucket(t *testing.T) {
	l := NewLimiter(2, 10*time.Second) // one token per 5s
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("k", t0); !ok {
			t.Fatalf("burst %d denied", i)
		}
	}
	ok, wait := l.Allow("k", t0)
	if ok || wait != 5*time.Second {
		t.Fatalf("third: ok=%v wait=%v", ok, wait)
	}
	ok, wait = l.Allow("k", t0.Add(2*time.Second))
	if ok || wait != 3*time.Second {
		t.Fatalf("at +2s: ok=%v wait=%v", ok, wait)
	}
	if ok, _ := l.Allow("k", t0.Add(5*time.Second)); !ok {
		t.Fatal("token should have refilled at +5s")
	}
	if ok, _ := l.Allow("k", t0.Add(5*time.Second)); ok {
		t.Fatal("only one token refilled")
	}
	// Long idle refills to capacity, not beyond.
	later := t0.Add(time.Hour)
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("k", later); !ok {
			t.Fatalf("after idle %d denied", i)
		}
	}
	if ok, _ := l.Allow("k", later); ok {
		t.Fatal("capacity exceeded after idle")
	}
}

func TestLimiterPerKeyIsolation(t *testing.T) {
	l := NewLimiter(1, time.Minute)
	if ok, _ := l.Allow("a", t0); !ok {
		t.Fatal("a")
	}
	if ok, _ := l.Allow("a", t0); ok {
		t.Fatal("a second")
	}
	if ok, _ := l.Allow("b", t0); !ok {
		t.Fatal("b must be unaffected")
	}
}

func TestLimiterSweep(t *testing.T) {
	l := NewLimiter(2, 10*time.Second)
	l.Allow("a", t0)
	l.Allow("b", t0)
	l.sweep(t0) // both below capacity
	if len(l.b) != 2 {
		t.Fatalf("swept busy buckets: %d", len(l.b))
	}
	l.sweep(t0.Add(10 * time.Second)) // both full again
	if len(l.b) != 0 {
		t.Fatalf("idle buckets kept: %d", len(l.b))
	}
}

func TestLimiterRunStops(t *testing.T) {
	l := NewLimiter(1, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { l.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not stop")
	}
}
