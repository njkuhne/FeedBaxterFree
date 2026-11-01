package ingest

import (
	"context"
	"math"
	"sync"
	"time"
)

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a token bucket per key ID: capacity perKey, refilled continuously
// at perKey tokens per window.
type Limiter struct {
	perKey int
	window time.Duration
	mu     sync.Mutex
	b      map[string]*bucket
}

// NewLimiter returns a limiter allowing perKey requests per window per key.
func NewLimiter(perKey int, window time.Duration) *Limiter {
	return &Limiter{perKey: perKey, window: window, b: map[string]*bucket{}}
}

// refilled returns the tokens b would hold at now.
func (l *Limiter) refilled(b *bucket, now time.Time) float64 {
	t := b.tokens
	if el := now.Sub(b.last); el > 0 {
		t += float64(el) * float64(l.perKey) / float64(l.window)
	}
	return math.Min(t, float64(l.perKey))
}

// Allow takes one token. When there is none it returns false and the wait
// until one is available.
func (l *Limiter) Allow(keyID string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.b[keyID]
	if !ok {
		b = &bucket{tokens: float64(l.perKey), last: now}
		l.b[keyID] = b
	}
	b.tokens = l.refilled(b, now)
	if now.After(b.last) {
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := (1 - b.tokens) * float64(l.window) / float64(l.perKey)
	return false, time.Duration(math.Ceil(wait))
}

// sweep deletes buckets that would be full at now.
func (l *Limiter) sweep(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.b {
		if l.refilled(b, now) >= float64(l.perKey) {
			delete(l.b, k)
		}
	}
}

// Run sweeps idle buckets every window until ctx is done.
func (l *Limiter) Run(ctx context.Context) {
	t := time.NewTicker(l.window)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			l.sweep(now)
		}
	}
}
