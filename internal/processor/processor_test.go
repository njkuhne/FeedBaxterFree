package processor_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/processor"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

const (
	waitTimeout  = 20 * time.Second
	pollInterval = 10 * time.Millisecond
)

func newTenant(t *testing.T, db *store.DB) (store.Tenant, store.Channel) {
	t.Helper()
	ctx := context.Background()
	tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, "acme", 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, "main", 1000)
	if err != nil {
		t.Fatal(err)
	}
	return tn, ch
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		<-tick.C
	}
}

func countRows(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM feedback").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// startProcessor runs p until the test ends and waits for Run to return.
func startProcessor(t *testing.T, p *processor.Processor) (stop func() time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	var stopped bool
	stop = func() time.Duration {
		if stopped {
			return 0
		}
		stopped = true
		start := time.Now()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run: %v", err)
			}
		case <-time.After(waitTimeout):
			t.Error("Run did not return")
		}
		return time.Since(start)
	}
	t.Cleanup(func() { stop() })
	return stop
}

type env struct {
	c      *queue.Conn
	o      *queue.Objects
	db     *store.DB
	tn     store.Tenant
	ch     store.Channel
	nextID int
}

func newEnv(t *testing.T, db *store.DB) *env {
	c, o := queuetest.Embedded(t, queue.Limits{})
	tn, ch := newTenant(t, db)
	return &env{c: c, o: o, db: db, tn: tn, ch: ch}
}

func (e *env) publish(t *testing.T, idem, msgID, body string) nats.Header {
	t.Helper()
	h := hdr(e.tn.ID, e.ch.ID, idem, time.Now())
	h.Set(jetstream.MsgIDHeader, msgID)
	_, err := e.c.JS.PublishMsg(context.Background(), &nats.Msg{Subject: queue.SubjectIngest, Header: h, Data: []byte(body)})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	return h
}

func (e *env) publishN(t *testing.T, n int) {
	t.Helper()
	for range n {
		e.nextID++
		id := fmt.Sprintf("k%d", e.nextID)
		e.publish(t, id, e.tn.ID+":"+id, fmt.Sprintf(`{"message":"feedback %d"}`, e.nextID))
	}
}

func (e *env) pending(t *testing.T) (ackPending, pending int) {
	t.Helper()
	ci, err := e.o.Consumer.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ci.NumAckPending, int(ci.NumPending)
}

// drained reports whether every message has been acked or terminated. It
// reads the stream, not the consumer: FEEDBACK is a work queue, so a message
// leaves it only when settled, and the count is exact the moment a publish is
// acknowledged. The consumer's pending counters can read zero for an instant
// after a publish, before the processor has fetched anything, which once made
// TestRedeliveredDuplicateHarmless see "drained" with no rows stored.
func (e *env) drained(t *testing.T) func() bool {
	return func() bool {
		si, err := e.o.Stream.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return si.State.Msgs == 0
	}
}

func (e *env) dlqCount(t *testing.T) int {
	t.Helper()
	si, err := e.o.DLQ.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return int(si.State.Msgs)
}

func (e *env) processor(cfg processor.Config) *processor.Processor {
	return processor.New(e.db, e.c.JS, e.o.Consumer, cfg, nil)
}

func TestBatchLandsAndAcks(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publishN(t, 50)
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "50 rows and drained consumer", func() bool {
			return countRows(t, db) == 50 && e.drained(t)()
		})
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d", n)
		}
		waitFor(t, "acked messages removed from the work-queue stream", func() bool {
			si, err := e.o.Stream.Info(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			return si.State.Msgs == 0
		})
	})
}

func TestRedeliveredDuplicateHarmless(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publish(t, "same", "id-a", `{"message":"once"}`)
		e.publish(t, "same", "id-b", `{"message":"once"}`)
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "drained", e.drained(t))
		if n := countRows(t, db); n != 1 {
			t.Errorf("rows = %d, want 1", n)
		}
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d", n)
		}
	})
}

func TestUnparseableGoesToDLQ(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publishN(t, 3)
		orig := e.publish(t, "bad", e.tn.ID+":bad", `{"message":"   "}`)
		e.publishN(t, 3)
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "6 rows, 1 dlq, drained", func() bool {
			return countRows(t, db) == 6 && e.dlqCount(t) == 1 && e.drained(t)()
		})

		raw, err := e.o.DLQ.GetLastMsgForSubject(context.Background(), queue.SubjectDLQ)
		if err != nil {
			t.Fatal(err)
		}
		if string(raw.Data) != `{"message":"   "}` {
			t.Errorf("dlq body = %q", raw.Data)
		}
		for k, v := range orig {
			if got := raw.Header.Values(k); len(got) != len(v) || got[0] != v[0] {
				t.Errorf("dlq header %s = %v, want %v", k, got, v)
			}
		}
		if got := raw.Header.Get(queue.HeaderError); got == "" || strings.ContainsAny(got, "\r\n") {
			t.Errorf("FB-Error = %q", got)
		}
	})
}

func TestStoreFailureEndsInDLQ(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		if _, err := db.SQL.Exec("ALTER TABLE feedback RENAME TO feedback_off"); err != nil {
			t.Fatal(err)
		}
		e.publish(t, "k1", "m1", `{"message":"doomed"}`)
		cfg := testCfg()
		cfg.MaxDeliver = 2
		startProcessor(t, e.processor(cfg))
		waitFor(t, "dlq copy and drained", func() bool { return e.dlqCount(t) == 1 && e.drained(t)() })

		raw, err := e.o.DLQ.GetLastMsgForSubject(context.Background(), queue.SubjectDLQ)
		if err != nil {
			t.Fatal(err)
		}
		if got := raw.Header.Get(queue.HeaderError); !strings.HasPrefix(got, "store:") {
			t.Errorf("FB-Error = %q", got)
		}
		if n := e.dlqCount(t); n != 1 {
			t.Errorf("dlq = %d", n)
		}
		var n int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM feedback_off").Scan(&n); err != nil || n != 0 {
			t.Errorf("rows in feedback_off = %d, err %v", n, err)
		}
	})
}

func TestShutdownFinishesBatch(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		const total = 300
		e := newEnv(t, db)
		e.publishN(t, total)
		cfg := testCfg()
		cfg.BatchSize = 20
		stop := startProcessor(t, e.processor(cfg))
		waitFor(t, "first rows", func() bool { return countRows(t, db) > 0 })
		if d := stop(); d > 5*time.Second {
			t.Errorf("Run took %s to return", d)
		}

		startProcessor(t, e.processor(cfg))
		waitFor(t, "all rows after restart", func() bool {
			return countRows(t, db) == total && e.drained(t)()
		})
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d", n)
		}
	})
}

func TestConcurrencyNoDuplicates(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publishN(t, 500)
		cfg := testCfg()
		cfg.BatchSize = 50
		cfg.Concurrency = 2
		if db.Dialect == store.Postgres {
			cfg.Concurrency = 4
		}
		startProcessor(t, e.processor(cfg))
		waitFor(t, "500 rows", func() bool { return countRows(t, db) == 500 && e.drained(t)() })
		var distinct int
		if err := db.SQL.QueryRow("SELECT COUNT(DISTINCT idempotency_key) FROM feedback").Scan(&distinct); err != nil || distinct != 500 {
			t.Errorf("distinct keys = %d, err %v", distinct, err)
		}
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d", n)
		}
	})
}

func TestPoisonRowIsolated(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publishN(t, 5)
		bad := hdr("tn_doesnotexist", e.ch.ID, "poison", time.Now())
		bad.Set(jetstream.MsgIDHeader, "poison")
		if _, err := e.c.JS.PublishMsg(context.Background(),
			&nats.Msg{Subject: queue.SubjectIngest, Header: bad, Data: []byte(`{"message":"orphan"}`)}); err != nil {
			t.Fatal(err)
		}
		e.publishN(t, 5)
		cfg := testCfg()
		cfg.MaxDeliver = 2
		startProcessor(t, e.processor(cfg))
		waitFor(t, "10 rows, poison in dlq, drained", func() bool {
			return countRows(t, db) == 10 && e.dlqCount(t) == 1 && e.drained(t)()
		})
		raw, err := e.o.DLQ.GetLastMsgForSubject(context.Background(), queue.SubjectDLQ)
		if err != nil {
			t.Fatal(err)
		}
		if got := raw.Header.Get(queue.HeaderError); !strings.HasPrefix(got, "store:") || strings.HasPrefix(got, "store: store:") {
			t.Errorf("FB-Error = %q", got)
		}
		if raw.Header.Get(queue.HeaderTenant) != "tn_doesnotexist" {
			t.Errorf("wrong message in dlq: %v", raw.Header)
		}
	})
}

// downDB is a database whose every connection attempt fails, standing in for
// an unreachable server on either dialect.
func downDB(d store.Dialect) *store.DB {
	return &store.DB{SQL: sql.OpenDB(downConnector{}), Dialect: d}
}

type downConnector struct{}

func (downConnector) Connect(context.Context) (driver.Conn, error) {
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
}
func (downConnector) Driver() driver.Driver { return nil }

func (e *env) delivered(t *testing.T) uint64 {
	t.Helper()
	ci, err := e.o.Consumer.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ci.Delivered.Consumer
}

func TestDatabaseOutageNeverDeadLetters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		e.publish(t, "k1", "m1", `{"message":"survives outage"}`)
		cfg := testCfg()
		cfg.MaxDeliver = 2
		p := processor.New(downDB(db.Dialect), e.c.JS, e.o.Consumer, cfg, nil)
		startProcessor(t, p)
		waitFor(t, "more than MaxDeliver+2 deliveries", func() bool { return e.delivered(t) > uint64(cfg.MaxDeliver)+2 })
		if n := e.dlqCount(t); n != 0 {
			t.Fatalf("dlq = %d during outage", n)
		}
		p.SetDB(db)
		waitFor(t, "row stored and drained", func() bool { return countRows(t, db) == 1 && e.drained(t)() })
		if n := e.dlqCount(t); n != 0 {
			t.Errorf("dlq = %d after recovery", n)
		}
	})
}

func TestDLQPublishFailureRetried(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		e := newEnv(t, db)
		ctx := context.Background()
		if err := e.c.JS.DeleteStream(ctx, queue.StreamDLQ); err != nil {
			t.Fatal(err)
		}
		if _, err := e.c.JS.PublishMsg(ctx, &nats.Msg{Subject: queue.SubjectDLQ, Data: []byte("x")}); err == nil {
			t.Skip("publishing to a deleted DLQ stream does not fail; cannot simulate")
		}
		e.publish(t, "bad", "bad", `not json`)
		startProcessor(t, e.processor(testCfg()))
		waitFor(t, "two deliveries", func() bool { return e.delivered(t) >= 2 })
		if a, p := e.pending(t); a+p == 0 {
			t.Fatal("message was settled without a DLQ copy")
		}
		if _, err := queue.Ensure(ctx, e.c.JS, queue.Limits{}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "dlq copy and drained", func() bool { return e.dlqCount(t) == 1 && e.drained(t)() })
	})
}

func TestBackoff(t *testing.T) {
	cfg := testCfg()
	cfg.NakDelay = time.Second
	p := processor.New(nil, nil, nil, cfg, nil)
	for _, tc := range []struct {
		delivered uint64
		want      time.Duration
	}{{1, time.Second}, {5, 5 * time.Second}, {30, 30 * time.Second}, {31, 30 * time.Second}, {1 << 40, 30 * time.Second}} {
		if got := p.Backoff(tc.delivered); got != tc.want {
			t.Errorf("Backoff(%d) = %s, want %s", tc.delivered, got, tc.want)
		}
	}
}

func TestPoisonRowDoesNotStallWorker(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		const stallLimit = 1500 * time.Millisecond
		e := newEnv(t, db)
		bad := hdr("tn_doesnotexist", e.ch.ID, "poison", time.Now())
		bad.Set(jetstream.MsgIDHeader, "poison")
		if _, err := e.c.JS.PublishMsg(context.Background(),
			&nats.Msg{Subject: queue.SubjectIngest, Header: bad, Data: []byte(`{"message":"orphan"}`)}); err != nil {
			t.Fatal(err)
		}
		cfg := testCfg()
		cfg.NakDelay = 2 * time.Second
		cfg.MaxDeliver = 5
		startProcessor(t, e.processor(cfg))
		waitFor(t, "poison delivered once", func() bool { return e.delivered(t) >= 1 })

		e.publishN(t, 20)
		deadline := time.Now().Add(stallLimit)
		tick := time.NewTicker(pollInterval)
		defer tick.Stop()
		for countRows(t, db) < 20 {
			if time.Now().After(deadline) {
				t.Fatalf("only %d of 20 rows within %s: worker stalled behind the poison row", countRows(t, db), stallLimit)
			}
			<-tick.C
		}
	})
}
