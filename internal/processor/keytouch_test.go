package processor_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func (e *env) newKey(t *testing.T) store.IngestKey {
	t.Helper()
	k, err := store.CreateIngestKey(context.Background(), e.db.SQL, e.db.Dialect, e.tn.ID, e.ch.ID, store.GenerateIngestKey(), 2000)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// publishAs publishes one message sent with key k, received at recv.
func (e *env) publishAs(t *testing.T, k store.IngestKey, idem string, recv time.Time) {
	t.Helper()
	h := hdr(e.tn.ID, e.ch.ID, idem, recv)
	h.Set(queue.HeaderKey, k.ID)
	h.Set(jetstream.MsgIDHeader, e.tn.ID+":"+idem)
	_, err := e.c.JS.PublishMsg(context.Background(), &nats.Msg{Subject: queue.SubjectIngest, Header: h, Data: []byte(`{"message":"hi"}`)})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *env) keyLastUsed(t *testing.T, k store.IngestKey) *int64 {
	t.Helper()
	got, err := store.GetIngestKey(context.Background(), e.db.SQL, e.db.Dialect, e.tn.ID, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got.LastUsedAt
}

func TestProcessorTouchesKeyLastUsed(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		k := e.newKey(t)
		old := time.Now().Add(-3 * time.Hour).Truncate(time.Millisecond)
		recent := old.Add(time.Hour)
		e.publishAs(t, k, "a", recent)
		e.publishAs(t, k, "b", old)
		// A message without a key header must not break the batch.
		e.publish(t, "nokey", e.tn.ID+":nokey", `{"message":"x"}`)
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "last_used_at", func() bool { return e.keyLastUsed(t, k) != nil })
		waitFor(t, "drained", e.drained(t))
		if got := e.keyLastUsed(t, k); *got != recent.UnixMilli() {
			t.Errorf("last_used_at = %d, want newest received time %d", *got, recent.UnixMilli())
		}
		if n := countRows(t, db); n != 3 {
			t.Errorf("rows = %d", n)
		}
	})
}

func TestProcessorTouchesKeyAtMostOncePerInterval(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		k := e.newKey(t)
		t1 := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
		t2 := t1.Add(time.Hour)
		t3 := t2.Add(time.Hour)

		cfg := testCfg()
		cfg.KeyTouchInterval = time.Hour
		stop := startProcessor(t, e.processor(cfg))
		e.publishAs(t, k, "one", t1)
		waitFor(t, "first touch", func() bool { return e.keyLastUsed(t, k) != nil })
		e.publishAs(t, k, "two", t2)
		waitFor(t, "second row", func() bool { return countRows(t, db) == 2 && e.drained(t)() })
		if got := e.keyLastUsed(t, k); *got != t1.UnixMilli() {
			t.Errorf("within interval: last_used_at = %d, want unchanged %d", *got, t1.UnixMilli())
		}
		stop()

		// Interval <= 0 touches on every batch.
		e2 := &env{c: e.c, o: e.o, db: db, tn: e.tn, ch: e.ch}
		cfg.KeyTouchInterval = 0
		startProcessor(t, e2.processor(cfg))
		e2.publishAs(t, k, "three", t3)
		waitFor(t, "touch with no interval", func() bool {
			got := e.keyLastUsed(t, k)
			return got != nil && *got == t3.UnixMilli()
		})
	})
}

func TestProcessorTouchFailureDoesNotAffectFeedback(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		k := e.newKey(t)
		// Make only the touch fail: the column is gone, feedback is untouched.
		if _, err := db.SQL.Exec("ALTER TABLE ingest_keys DROP COLUMN last_used_at"); err != nil {
			t.Fatalf("drop last_used_at: %v", err)
		}
		e.publishAs(t, k, "a", time.Now())
		e.publishAs(t, k, "b", time.Now())
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "rows stored and acked", func() bool { return countRows(t, db) == 2 && e.drained(t)() })
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d", n)
		}
		if ap, _ := e.pending(t); ap != 0 {
			t.Errorf("ack pending = %d", ap)
		}
		if d := e.delivered(t); d != 2 {
			t.Errorf("deliveries = %d, want 2 (one per message, no redelivery)", d)
		}
	})
}
