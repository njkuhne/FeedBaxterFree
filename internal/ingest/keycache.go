package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// rewatchDelay is the pause before re-establishing a watch that ended while ctx is live.
const rewatchDelay = time.Second

// TenantWindow is a tenant and its billing day, as Counters.Load needs it.
type TenantWindow struct {
	TenantID          string
	BillingMonthStart int
}

// KeyCache is the in-memory copy of the `keys` bucket.
type KeyCache struct {
	log       *slog.Logger
	mu        sync.RWMutex
	entries   map[string]Entry
	ready     chan struct{}
	readyOnce sync.Once
}

// NewKeyCache returns an empty, not-yet-ready cache.
func NewKeyCache(log *slog.Logger) *KeyCache {
	if log == nil {
		log = slog.Default()
	}
	return &KeyCache{log: log, entries: map[string]Entry{}, ready: make(chan struct{})}
}

// Run watches kv with WatchAll(IncludeHistory) until ctx is done. Put decodes
// and stores the entry (malformed JSON: warn, drop that key); Delete and Purge
// remove it. The nil entry that ends the initial replay closes Ready. If the
// watch ends while ctx is live it is re-established after rewatchDelay. It
// returns nil when ctx is cancelled.
func (c *KeyCache) Run(ctx context.Context, kv jetstream.KeyValue) error {
	for {
		if err := c.watchOnce(ctx, kv); err != nil && ctx.Err() == nil {
			c.log.Warn("ingest: key watch ended", "err", err)
		}
		if ctx.Err() != nil {
			return nil
		}
		t := time.NewTimer(rewatchDelay)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

func (c *KeyCache) watchOnce(ctx context.Context, kv jetstream.KeyValue) error {
	w, err := kv.WatchAll(ctx, jetstream.IncludeHistory())
	if err != nil {
		return err
	}
	defer w.Stop()
	seen := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-w.Updates():
			if !ok {
				return nil
			}
			if e == nil { // end of initial replay
				c.pruneUnseen(seen)
				c.readyOnce.Do(func() { close(c.ready) })
				continue
			}
			seen[e.Key()] = struct{}{}
			c.apply(e)
		}
	}
}

func (c *KeyCache) apply(e jetstream.KeyValueEntry) {
	switch e.Operation() {
	case jetstream.KeyValueDelete, jetstream.KeyValuePurge:
		c.mu.Lock()
		delete(c.entries, e.Key())
		c.mu.Unlock()
		return
	}
	var ent Entry
	if err := json.Unmarshal(e.Value(), &ent); err != nil {
		c.log.Warn("ingest: malformed key entry removed", "err", err)
		c.mu.Lock()
		delete(c.entries, e.Key())
		c.mu.Unlock()
		return
	}
	c.mu.Lock()
	c.entries[e.Key()] = ent
	c.mu.Unlock()
}

// pruneUnseen drops entries a (re)played watch did not mention: they were
// removed while no watch was running.
func (c *KeyCache) pruneUnseen(seen map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		if _, ok := seen[k]; !ok {
			delete(c.entries, k)
		}
	}
}

// Lookup returns the entry for a key hash.
func (c *KeyCache) Lookup(hash string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[hash]
	return e, ok
}

// Ready is closed once the initial replay has been applied.
func (c *KeyCache) Ready() <-chan struct{} { return c.ready }

// Tenants returns each distinct tenant once, with its billing day.
func (c *KeyCache) Tenants() []TenantWindow {
	c.mu.RLock()
	defer c.mu.RUnlock()
	seen := map[string]struct{}{}
	var out []TenantWindow
	for _, e := range c.entries {
		if _, ok := seen[e.TenantID]; ok {
			continue
		}
		seen[e.TenantID] = struct{}{}
		out = append(out, TenantWindow{TenantID: e.TenantID, BillingMonthStart: e.BillingMonthStart})
	}
	return out
}
