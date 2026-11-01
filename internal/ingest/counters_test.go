package ingest

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
)

const flushHour = time.Hour // Run's timer is never what a test waits on

func newCounters(c *queue.Conn, o *queue.Objects) *Counters {
	return NewCounters(c.JS, o.Counters, flushHour, discardLog())
}

func kvInt(t *testing.T, kv jetstream.KeyValue, key string) int64 {
	t.Helper()
	e, err := kv.Get(context.Background(), key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	v, err := strconv.ParseInt(string(e.Value()), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCountersKeyNames(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 23, 59, 0, 0, time.UTC) }
	cases := []struct {
		billing int
		now     time.Time
		want    string
	}{
		{1, d(2026, 3, 10), "t.month.2026-03-01"},
		{0, d(2026, 3, 10), "t.month.2026-03-01"},
		{15, d(2026, 3, 10), "t.month.2026-02-15"},
		{15, d(2026, 3, 15), "t.month.2026-03-15"},
		{15, d(2026, 3, 31), "t.month.2026-03-15"},
		{28, d(2026, 1, 5), "t.month.2025-12-28"},
		{28, d(2026, 3, 28), "t.month.2026-03-28"},
		{28, d(2026, 3, 27), "t.month.2026-02-28"},
	}
	for _, c := range cases {
		if got := monthKey("t", c.billing, c.now); got != c.want {
			t.Errorf("billing %d at %s: got %s want %s", c.billing, c.now.Format(dateLayout), got, c.want)
		}
	}
	if got := dayKey("t", time.Date(2026, 3, 10, 1, 0, 0, 0, time.FixedZone("x", 5*3600))); got != "t.day.2026-03-09" {
		t.Errorf("day key must use UTC: %s", got)
	}
}

func TestCountersDaily(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	cn := newCounters(c, o)
	now := time.Date(2026, 3, 10, 23, 0, 0, 0, time.UTC)
	lim := ext.Limits{Daily: 2}
	for i := 0; i < 2; i++ {
		if v := cn.TryIncr("t", lim, 1, now); v.Result != Allowed {
			t.Fatalf("%d: %v", i, v.Result)
		}
	}
	v := cn.TryIncr("t", lim, 1, now)
	if v.Result != DailyLimited || v.RetryAfter != time.Hour {
		t.Fatalf("third: %+v", v)
	}
	// A new UTC day starts a new counter.
	if v := cn.TryIncr("t", lim, 1, now.Add(time.Hour)); v.Result != Allowed {
		t.Fatalf("next day: %v", v.Result)
	}
	// Unlimited still counts.
	for i := 0; i < 5; i++ {
		cn.TryIncr("u", ext.Limits{}, 1, now)
	}
	if s := cn.Snapshot()["u"]; s != [2]int64{5, 5} {
		t.Fatalf("unlimited snapshot: %v", s)
	}
}

func TestCountersMonthly(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	cn := newCounters(c, o)
	now := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC)
	lim := ext.Limits{Monthly: 2}
	cn.TryIncr("t", lim, 1, now)
	cn.TryIncr("t", lim, 1, now)
	if v := cn.TryIncr("t", lim, 1, now); v.Result != PlanLimited {
		t.Fatalf("third: %v", v.Result)
	}
	// Later the same billing month, next day: still limited.
	if v := cn.TryIncr("t", lim, 1, now.Add(24*time.Hour)); v.Result != PlanLimited {
		t.Fatalf("next day: %v", v.Result)
	}
	// Billing period rolls on the 15th for billing day 15, so day 15 is a new month.
	if v := cn.TryIncr("t", lim, 15, time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)); v.Result != Allowed {
		t.Fatalf("new period: %v", v.Result)
	}
}

func TestCountersPlanWins(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	cn := newCounters(c, o)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	lim := ext.Limits{Daily: 1, Monthly: 1}
	cn.TryIncr("t", lim, 1, now)
	if v := cn.TryIncr("t", lim, 1, now); v.Result != PlanLimited {
		t.Fatalf("both exceeded: %v", v.Result)
	}
}

func TestCountersRefund(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	cn := newCounters(c, o)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	v := cn.TryIncr("t", ext.Limits{Daily: 1}, 1, now)
	cn.Refund(v)
	if s := cn.Snapshot()["t"]; s != [2]int64{} {
		t.Fatalf("after refund: %v", s)
	}
	if v := cn.TryIncr("t", ext.Limits{Daily: 1}, 1, now); v.Result != Allowed {
		t.Fatal("refund must free the slot")
	}
	if err := cn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	// Refunding an already-flushed increment is written by the next flush.
	cn.Refund(Verdict{Result: Allowed, tenantID: "t", dayKey: dayKey("t", now), monthKey: monthKey("t", 1, now)})
	if err := cn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := kvInt(t, o.Counters, dayKey("t", now)); got != 0 {
		t.Fatalf("kv after refund flush: %d", got)
	}
	// Refunding a non-Allowed verdict is a no-op.
	cn.Refund(Verdict{Result: DailyLimited})
}

func TestCountersFlushAndLoad(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	a := newCounters(c, o)
	for i := 0; i < 3; i++ {
		a.TryIncr("t", ext.Limits{}, 1, now)
	}
	if err := a.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	dk, mk := dayKey("t", now), monthKey("t", 1, now)
	if kvInt(t, o.Counters, dk) != 3 || kvInt(t, o.Counters, mk) != 3 {
		t.Fatal("kv values after flush")
	}
	st, err := c.JS.Stream(ctx, "KV_"+queue.BucketCounters)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{dk, mk} {
		raw, err := st.GetLastMsgForSubject(ctx, "$KV."+queue.BucketCounters+"."+k)
		if err != nil {
			t.Fatal(err)
		}
		if raw.Header.Get("Nats-TTL") == "" {
			t.Fatalf("%s: no TTL header: %v", k, raw.Header)
		}
	}
	// A second flush writes via the CAS-update path (rev != 0) and keeps the TTL.
	a.TryIncr("t", ext.Limits{}, 1, now)
	if err := a.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := st.GetLastMsgForSubject(ctx, "$KV."+queue.BucketCounters+"."+dk)
	if err != nil || raw.Header.Get("Nats-TTL") == "" || kvInt(t, o.Counters, dk) != 4 {
		t.Fatalf("update path: err=%v hdr=%v", err, raw.Header)
	}

	b := newCounters(c, o)
	if err := b.Load(ctx, []TenantWindow{{TenantID: "t", BillingMonthStart: 1}}, now); err != nil {
		t.Fatal(err)
	}
	if s := b.Snapshot()["t"]; s != [2]int64{4, 4} {
		t.Fatalf("loaded: %v", s)
	}
	// Load makes the limit check see the shared value.
	if v := b.TryIncr("t", ext.Limits{Daily: 5}, 1, now); v.Result != Allowed {
		t.Fatalf("5th: %v", v.Result)
	}
	if v := b.TryIncr("t", ext.Limits{Daily: 5}, 1, now); v.Result != DailyLimited {
		t.Fatalf("6th: %v", v.Result)
	}
}

func TestCountersTwoReplicasConverge(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	a, b := newCounters(c, o), newCounters(c, o)
	lim := ext.Limits{}
	incr := func(cn *Counters, n int) {
		for i := 0; i < n; i++ {
			cn.TryIncr("t", lim, 1, now)
		}
	}
	flush := func(cn *Counters) {
		t.Helper()
		if err := cn.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}
	incr(a, 3)
	incr(b, 4)
	flush(a)
	flush(b)
	incr(a, 2)

	// Force a conflict: while a is between its read and its write, b flushes.
	var once sync.Once
	a.beforeWrite = func(string) {
		once.Do(func() {
			incr(b, 5)
			flush(b)
		})
	}
	flush(a)
	a.beforeWrite = nil
	flush(b)
	flush(a)

	const total = 3 + 4 + 2 + 5
	dk := dayKey("t", now)
	if got := kvInt(t, o.Counters, dk); got != total {
		t.Fatalf("kv day = %d, want %d", got, total)
	}
	if kvInt(t, o.Counters, monthKey("t", 1, now)) != total {
		t.Fatal("kv month")
	}
	sa, sb := a.Snapshot()["t"], b.Snapshot()["t"]
	if sa != [2]int64{total, total} || sb != sa {
		t.Fatalf("snapshots: a=%v b=%v", sa, sb)
	}
}

func TestCountersFlushGivesUpKeepsDelta(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	a := newCounters(c, o)
	for i := 0; i < 3; i++ {
		a.TryIncr("t", ext.Limits{}, 1, now)
	}
	dk := dayKey("t", now)
	var calls atomic.Int32
	a.beforeWrite = func(key string) { // someone else always wins the race
		calls.Add(1)
		if _, err := o.Counters.Put(ctx, key, []byte("10")); err != nil {
			t.Error(err)
		}
	}
	if err := a.Flush(ctx); err == nil {
		t.Fatal("want an error after repeated conflicts")
	}
	if got := calls.Load(); got < 2*maxFlushAttempts { // day and month keys
		t.Fatalf("attempts: %d", got)
	}
	if s := a.Snapshot()["t"]; s != [2]int64{3, 3} {
		t.Fatalf("delta lost or doubled: %v", s)
	}
	a.beforeWrite = nil
	if err := a.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := kvInt(t, o.Counters, dk); got != 13 {
		t.Fatalf("kv = %d, want 13", got)
	}
	if s := a.Snapshot()["t"]; s != [2]int64{13, 13} {
		t.Fatalf("snapshot: %v", s)
	}
}

func TestCountersConcurrentNoLoss(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cn := newCounters(c, o)
	var net atomic.Int64
	var wg sync.WaitGroup
	stop := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		for {
			if err := cn.Flush(ctx); err != nil {
				t.Error(err)
				return
			}
			select {
			case <-stop:
				return
			default:
			}
		}
	}()
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				v := cn.TryIncr("t", ext.Limits{}, 1, now)
				if v.Result != Allowed {
					t.Error("unexpected rejection")
					return
				}
				if i%3 == 0 {
					cn.Refund(v)
				} else {
					net.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-flushed
	if err := cn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if got := kvInt(t, o.Counters, dayKey("t", now)); got != net.Load() {
		t.Fatalf("kv = %d, accepted = %d", got, net.Load())
	}
	if s := cn.Snapshot()["t"]; s[0] != net.Load() {
		t.Fatalf("snapshot %v", s)
	}
}

func TestCountersOvershootBound(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	const L = 5
	lim := ext.Limits{Daily: L}
	a, b := newCounters(c, o), newCounters(c, o)
	drain := func(cn *Counters) int {
		n := 0
		for cn.TryIncr("t", lim, 1, now).Result == Allowed {
			n++
		}
		return n
	}
	na := drain(a)
	nb := drain(b)
	if na != L || nb != L {
		t.Fatalf("each replica accepts up to the limit between flushes: %d %d", na, nb)
	}
	// total accepted <= L + what the other replica accepted since the last flush
	if na+nb > L+nb {
		t.Fatalf("overshoot beyond bound: %d", na+nb)
	}
	for _, cn := range []*Counters{a, b, a} {
		if err := cn.Flush(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := kvInt(t, o.Counters, dayKey("t", now)); got != int64(na+nb) {
		t.Fatalf("kv = %d", got)
	}
	for name, cn := range map[string]*Counters{"a": a, "b": b} {
		if v := cn.TryIncr("t", lim, 1, now); v.Result != DailyLimited {
			t.Fatalf("%s accepts after flush: %v", name, v.Result)
		}
	}
}

func TestCountersRunFinalFlush(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cn := newCounters(c, o)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { cn.Run(ctx); close(done) }()
	cn.TryIncr("t", ext.Limits{}, 1, now)
	cn.TryIncr("t", ext.Limits{}, 1, now)
	cancel()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Run did not return")
	}
	if got := kvInt(t, o.Counters, dayKey("t", now)); got != 2 {
		t.Fatalf("kv = %d, want 2", got)
	}
}

func TestCountersDropsPastWindows(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	cn := newCounters(c, o)
	day1 := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	cn.TryIncr("t", ext.Limits{}, 1, day1)
	if err := cn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	cn.TryIncr("t", ext.Limits{}, 1, day1.Add(24*time.Hour))
	if err := cn.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	ts := cn.tenant("t")
	if _, ok := ts.ctrs[dayKey("t", day1)]; ok {
		t.Fatal("past day counter with zero delta should be dropped")
	}
}
