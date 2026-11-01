package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
)

const ttlHeader = "Nats-TTL"

func TestPutTTL(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	kv := o.Counters

	rev, err := queue.PutTTL(ctx, c.JS, kv, "d.t1", []byte("1"), 0, time.Hour)
	if err != nil || rev == 0 {
		t.Fatalf("create: rev %d err %v", rev, err)
	}
	if _, err := queue.PutTTL(ctx, c.JS, kv, "d.t1", []byte("x"), 0, time.Hour); !queue.IsWrongLastSequence(err) {
		t.Fatalf("second create: want wrong last sequence, got %v", err)
	}
	if _, err := queue.PutTTL(ctx, c.JS, kv, "d.t1", []byte("x"), rev+5, time.Hour); !queue.IsWrongLastSequence(err) {
		t.Fatalf("stale rev: want wrong last sequence, got %v", err)
	}
	rev2, err := queue.PutTTL(ctx, c.JS, kv, "d.t1", []byte("2"), rev, time.Hour)
	if err != nil || rev2 <= rev {
		t.Fatalf("update: rev %d err %v", rev2, err)
	}
	e, err := kv.Get(ctx, "d.t1")
	if err != nil || string(e.Value()) != "2" {
		t.Fatalf("get: %v %v", e, err)
	}

	st, err := c.JS.Stream(ctx, "KV_"+queue.BucketCounters)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := st.GetLastMsgForSubject(ctx, "$KV."+queue.BucketCounters+".d.t1")
	if err != nil {
		t.Fatal(err)
	}
	if raw.Header.Get(ttlHeader) == "" {
		t.Fatalf("no %s header: %v", ttlHeader, raw.Header)
	}
}

func TestPutTTLExpires(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	if _, err := queue.PutTTL(ctx, c.JS, o.Counters, "k", []byte("1"), 0, time.Second); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, err := o.Counters.Get(ctx, "k")
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("key not expired, last err %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := queue.PutTTL(ctx, c.JS, o.Counters, "k", []byte("2"), 0, time.Hour); err != nil {
		t.Fatalf("create after expiry: %v", err)
	}
}
