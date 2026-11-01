package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/queue"
)

const (
	// maxFlushAttempts bounds the CAS retries per key in one Flush.
	maxFlushAttempts = 5
	// finalFlushTimeout bounds the flush Run does after ctx is cancelled.
	finalFlushTimeout = 5 * time.Second

	dateLayout = "2006-01-02"
)

// Result is the outcome of Counters.TryIncr.
type Result int

const (
	Allowed Result = iota
	DailyLimited
	PlanLimited
)

// Verdict is what TryIncr decided. Pass an Allowed verdict to Refund to undo it.
type Verdict struct {
	Result     Result
	RetryAfter time.Duration // DailyLimited only
	tenantID   string
	dayKey     string
	monthKey   string
}

// counter is one KV key: shared is the last value seen in KV, delta what this
// replica has added since and not yet written. Guarded by tenantState.mu.
type counter struct {
	ttl           time.Duration
	shared, delta int64
}

type tenantState struct {
	mu               sync.Mutex
	billingDay       int
	curDay, curMonth string // keys of the most recent window used
	ctrs             map[string]*counter
}

func (ts *tenantState) get(key string, ttl time.Duration) *counter {
	c, ok := ts.ctrs[key]
	if !ok {
		c = &counter{ttl: ttl}
		ts.ctrs[key] = c
	}
	return c
}

// Counters keeps per-tenant day and month counts: a local delta merged into
// the shared KV value by compare-and-swap on a timer.
type Counters struct {
	js         jetstream.JetStream
	kv         jetstream.KeyValue
	flushEvery time.Duration
	log        *slog.Logger

	mu      sync.RWMutex
	tenants map[string]*tenantState

	flushMu sync.Mutex // one Flush at a time, or a delta could be applied twice

	beforeWrite func(key string) // test seam: runs between a flush's read and its write
}

// NewCounters returns counters over kv (the `counters` bucket).
func NewCounters(js jetstream.JetStream, kv jetstream.KeyValue, flushEvery time.Duration, log *slog.Logger) *Counters {
	if log == nil {
		log = slog.Default()
	}
	return &Counters{js: js, kv: kv, flushEvery: flushEvery, log: log, tenants: map[string]*tenantState{}}
}

func (c *Counters) tenant(id string) *tenantState {
	c.mu.RLock()
	ts, ok := c.tenants[id]
	c.mu.RUnlock()
	if ok {
		return ts
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if ts, ok = c.tenants[id]; !ok {
		ts = &tenantState{ctrs: map[string]*counter{}}
		c.tenants[id] = ts
	}
	return ts
}

func clampDay(b int) int {
	switch {
	case b < 1:
		return 1
	case b > 28:
		return 28
	}
	return b
}

func dayKey(tenantID string, now time.Time) string {
	return tenantID + ".day." + now.UTC().Format(dateLayout)
}

// monthKey names the billing period containing now: day b of this UTC month
// if today's day >= b, else day b of the previous month.
func monthKey(tenantID string, billingDay int, now time.Time) string {
	u := now.UTC()
	b := clampDay(billingDay)
	y, m, d := u.Date()
	if d < b {
		m--
	}
	return tenantID + ".month." + time.Date(y, m, b, 0, 0, 0, 0, time.UTC).Format(dateLayout)
}

// Load reads the current day and month values of each tenant before serving.
func (c *Counters) Load(ctx context.Context, tenants []TenantWindow, now time.Time) error {
	for _, tw := range tenants {
		ts := c.tenant(tw.TenantID)
		dk, mk := dayKey(tw.TenantID, now), monthKey(tw.TenantID, tw.BillingMonthStart, now)
		for _, k := range []struct {
			key string
			ttl time.Duration
		}{{dk, queue.CounterDayTTL}, {mk, queue.CounterMonthTTL}} {
			val, _, err := c.read(ctx, k.key)
			if err != nil {
				return fmt.Errorf("ingest: load counter %s: %w", k.key, err)
			}
			ts.mu.Lock()
			ts.get(k.key, k.ttl).shared = val
			ts.mu.Unlock()
		}
		ts.mu.Lock()
		ts.billingDay, ts.curDay, ts.curMonth = tw.BillingMonthStart, dk, mk
		ts.mu.Unlock()
	}
	return nil
}

// read returns the KV value and revision of key; a missing key is (0, 0).
func (c *Counters) read(ctx context.Context, key string) (int64, uint64, error) {
	e, err := c.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	v, err := strconv.ParseInt(string(e.Value()), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad counter value %q: %w", e.Value(), err)
	}
	return v, e.Revision(), nil
}

// TryIncr counts one submission unless a limit is reached. The check uses the
// shared value plus the local delta. PlanLimited wins over DailyLimited: it
// means "stop retrying", whereas daily would send SDKs back at midnight into
// a still-full month.
func (c *Counters) TryIncr(tenantID string, lim ext.Limits, billingDay int, now time.Time) Verdict {
	ts := c.tenant(tenantID)
	dk, mk := dayKey(tenantID, now), monthKey(tenantID, billingDay, now)
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.billingDay, ts.curDay, ts.curMonth = billingDay, dk, mk
	day, month := ts.get(dk, queue.CounterDayTTL), ts.get(mk, queue.CounterMonthTTL)
	if lim.Monthly > 0 && month.shared+month.delta+1 > lim.Monthly {
		return Verdict{Result: PlanLimited}
	}
	if lim.Daily > 0 && day.shared+day.delta+1 > lim.Daily {
		u := now.UTC()
		next := time.Date(u.Year(), u.Month(), u.Day()+1, 0, 0, 0, 0, time.UTC)
		return Verdict{Result: DailyLimited, RetryAfter: next.Sub(now)}
	}
	day.delta++
	month.delta++
	return Verdict{Result: Allowed, tenantID: tenantID, dayKey: dk, monthKey: mk}
}

// Refund undoes an Allowed verdict, so only accepted submissions count.
func (c *Counters) Refund(v Verdict) {
	if v.Result != Allowed {
		return
	}
	ts := c.tenant(v.tenantID)
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.get(v.dayKey, queue.CounterDayTTL).delta--
	ts.get(v.monthKey, queue.CounterMonthTTL).delta--
}

type flushItem struct {
	ts   *tenantState
	key  string
	ctr  *counter
	past bool
}

// Flush merges every local delta into KV: Get, then a CAS write of value+delta
// carrying the key's TTL. On conflict it re-reads and retries up to
// maxFlushAttempts; after that the delta stays for the next flush. Only the
// amount that was written is subtracted, so increments made during the round
// trip are kept. Counters with no delta just refresh their shared value, which
// is how replicas see each other. It tries every key and returns the first error.
func (c *Counters) Flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	var items []flushItem
	c.mu.RLock()
	for _, ts := range c.tenants {
		ts.mu.Lock()
		for k, ctr := range ts.ctrs {
			items = append(items, flushItem{ts: ts, key: k, ctr: ctr, past: k != ts.curDay && k != ts.curMonth})
		}
		ts.mu.Unlock()
	}
	c.mu.RUnlock()

	var first error
	for _, it := range items {
		if it.past {
			it.ts.mu.Lock()
			if it.ctr.delta == 0 && it.key != it.ts.curDay && it.key != it.ts.curMonth {
				delete(it.ts.ctrs, it.key)
				it.ts.mu.Unlock()
				continue
			}
			it.ts.mu.Unlock()
		}
		if err := c.flushOne(ctx, it); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (c *Counters) flushOne(ctx context.Context, it flushItem) error {
	for attempt := 0; attempt < maxFlushAttempts; attempt++ {
		it.ts.mu.Lock()
		d := it.ctr.delta
		it.ts.mu.Unlock()

		val, rev, err := c.read(ctx, it.key)
		if err != nil {
			return fmt.Errorf("ingest: flush %s: %w", it.key, err)
		}
		if d == 0 {
			it.ts.mu.Lock()
			it.ctr.shared = val
			it.ts.mu.Unlock()
			return nil
		}
		if c.beforeWrite != nil {
			c.beforeWrite(it.key)
		}
		_, err = queue.PutTTL(ctx, c.js, c.kv, it.key, []byte(strconv.FormatInt(val+d, 10)), rev, it.ctr.ttl)
		if err == nil {
			it.ts.mu.Lock()
			it.ctr.shared = val + d
			it.ctr.delta -= d
			it.ts.mu.Unlock()
			return nil
		}
		if !queue.IsWrongLastSequence(err) {
			return fmt.Errorf("ingest: flush %s: %w", it.key, err)
		}
	}
	c.log.Warn("ingest: counter flush kept its delta after repeated conflicts", "key", it.key)
	return fmt.Errorf("ingest: flush %s: %d conflicts in a row", it.key, maxFlushAttempts)
}

// Run flushes every flushEvery until ctx is done, then flushes once more on a
// context detached from ctx.
func (c *Counters) Run(ctx context.Context) {
	t := time.NewTicker(c.flushEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalFlushTimeout)
			defer cancel()
			if err := c.Flush(fctx); err != nil {
				c.log.Warn("ingest: final counter flush", "err", err)
			}
			return
		case <-t.C:
			if err := c.Flush(ctx); err != nil && ctx.Err() == nil {
				c.log.Warn("ingest: counter flush", "err", err)
			}
		}
	}
}

// Snapshot returns each tenant's local (day, month) totals, shared+delta.
func (c *Counters) Snapshot() map[string][2]int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string][2]int64, len(c.tenants))
	for id, ts := range c.tenants {
		ts.mu.Lock()
		var v [2]int64
		if d, ok := ts.ctrs[ts.curDay]; ok {
			v[0] = d.shared + d.delta
		}
		if m, ok := ts.ctrs[ts.curMonth]; ok {
			v[1] = m.shared + m.delta
		}
		ts.mu.Unlock()
		out[id] = v
	}
	return out
}
