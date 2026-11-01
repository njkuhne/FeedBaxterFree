package processor

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/store"
)

const (
	retentionBatch = 1000
	jitterSpread   = 0.1 // interval is scaled by a uniform factor in [1-spread, 1+spread]
)

// RunRetention deletes rows received before now-RetentionWindow, one
// transaction per batch, and returns how many it deleted.
func (p *Processor) RunRetention(ctx context.Context, now time.Time) (int64, error) {
	cutoff := now.Add(-p.cfg.RetentionWindow).UnixMilli()
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := p.deleteBatch(ctx, cutoff)
		total += n
		if err != nil {
			return total, err
		}
		if n < retentionBatch {
			return total, nil
		}
	}
}

func (p *Processor) deleteBatch(ctx context.Context, cutoff int64) (int64, error) {
	tx, err := p.db.Load().SQL.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("retention: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	n, err := store.DeleteFeedbackOlderThan(ctx, tx, p.db.Load().Dialect, cutoff, retentionBatch)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("retention: commit: %w", err)
	}
	return n, nil
}

func (p *Processor) retentionLoop(ctx context.Context) {
	if p.cfg.RetentionCheckInterval <= 0 {
		return
	}
	for {
		wait := time.Duration(float64(p.cfg.RetentionCheckInterval) * (1 - jitterSpread + 2*jitterSpread*rand.Float64()))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		n, err := p.RunRetention(ctx, time.Now())
		if err != nil && ctx.Err() == nil {
			p.log.Warn("retention failed", "deleted", n, "err", err)
		}
		if n > 0 {
			p.log.Info("retention deleted old feedback", "rows", n)
		}
	}
}
