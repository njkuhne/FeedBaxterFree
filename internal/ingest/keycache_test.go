package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
)

func TestKeyCacheReplayThenReady(t *testing.T) {
	_, o := queuetest.Embedded(t, queue.Limits{})
	putEntry(t, o.Keys, "k1", Entry{KeyID: "key_1", TenantID: "tn_1", ChannelID: "ch_1", DailyLimit: 3})
	putEntry(t, o.Keys, "k2", Entry{KeyID: "key_2", TenantID: "tn_2", ChannelID: "ch_2", BillingMonthStart: 15})
	c := startCache(t, o.Keys)
	e, ok := c.Lookup(HashKey("k1"))
	if !ok || e.KeyID != "key_1" || e.DailyLimit != 3 {
		t.Fatalf("k1: %+v %v", e, ok)
	}
	if _, ok := c.Lookup(HashKey("k2")); !ok {
		t.Fatal("k2 missing at ready")
	}
	if _, ok := c.Lookup(HashKey("nope")); ok {
		t.Fatal("unexpected hit")
	}
	if got := len(c.Tenants()); got != 2 {
		t.Fatalf("tenants: %d", got)
	}
}

func TestKeyCacheLiveUpdates(t *testing.T) {
	_, o := queuetest.Embedded(t, queue.Limits{})
	c := startCache(t, o.Keys)
	h := HashKey("k")

	putEntry(t, o.Keys, "k", Entry{KeyID: "key_1", TenantID: "tn_1"})
	eventually(t, "create", func() bool { _, ok := c.Lookup(h); return ok })

	putEntry(t, o.Keys, "k", Entry{KeyID: "key_1", TenantID: "tn_1", Revoked: true})
	eventually(t, "revoke", func() bool { e, _ := c.Lookup(h); return e.Revoked })

	putEntry(t, o.Keys, "k", Entry{KeyID: "key_1", TenantID: "tn_1", DailyLimit: 9})
	eventually(t, "limit change", func() bool { e, _ := c.Lookup(h); return e.DailyLimit == 9 && !e.Revoked })

	if err := o.Keys.Delete(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	eventually(t, "delete", func() bool { _, ok := c.Lookup(h); return !ok })

	putEntry(t, o.Keys, "k2", Entry{KeyID: "key_2", TenantID: "tn_1"})
	eventually(t, "second create", func() bool { _, ok := c.Lookup(HashKey("k2")); return ok })
	if err := o.Keys.Purge(context.Background(), HashKey("k2")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "purge", func() bool { _, ok := c.Lookup(HashKey("k2")); return !ok })
}

func TestKeyCacheEmptyBucketReady(t *testing.T) {
	_, o := queuetest.Embedded(t, queue.Limits{})
	c := startCache(t, o.Keys) // fails if Ready never closes
	if len(c.Tenants()) != 0 {
		t.Fatal("expected no tenants")
	}
}

func TestKeyCacheMalformed(t *testing.T) {
	_, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	putEntry(t, o.Keys, "k", Entry{KeyID: "key_1", TenantID: "tn_1"})
	if _, err := o.Keys.Put(ctx, HashKey("bad"), []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	c := startCache(t, o.Keys)
	if _, ok := c.Lookup(HashKey("bad")); ok {
		t.Fatal("malformed entry cached")
	}
	if _, ok := c.Lookup(HashKey("k")); !ok {
		t.Fatal("good entry missing")
	}
	// A good entry later overwritten by garbage is removed.
	if _, err := o.Keys.Put(ctx, HashKey("k"), []byte("garbage")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "removal of overwritten entry", func() bool { _, ok := c.Lookup(HashKey("k")); return !ok })
}

func TestKeyCacheRunStopsOnCancel(t *testing.T) {
	_, o := queuetest.Embedded(t, queue.Limits{})
	c := NewKeyCache(discardLog())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, o.Keys) }()
	<-c.Ready()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Run did not stop")
	}
}
