// Package keysync writes the ingest key cache (the `keys` KV bucket) from the
// database. It is separate from package ingest so that package stays DB-free.
package keysync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/ingest"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/store"
)

var _ ext.KeyWriter = (*Writer)(nil)

// Writer syncs ingest keys from DB to KV.
type Writer struct {
	DB     *store.DB
	KV     jetstream.KeyValue
	Limits ext.LimitsSource
	Log    *slog.Logger

	// Test seams, called inside Reconcile after the KV snapshot and after the DB read.
	afterSnapshot, afterDBRead func()
}

// Stats counts what a Reconcile changed.
type Stats struct{ Put, Deleted int }

// EntryFor builds the cache entry of a key row.
func EntryFor(k store.CacheKey, lim ext.Limits) ingest.Entry {
	return ingest.Entry{
		KeyID: k.KeyID, TenantID: k.TenantID, ChannelID: k.ChannelID,
		DailyLimit: lim.Daily, MonthlyLimit: lim.Monthly,
		Revoked: k.Revoked, BillingMonthStart: k.BillingMonthStart,
	}
}

func (w *Writer) log() *slog.Logger {
	if w.Log != nil {
		return w.Log
	}
	return slog.Default()
}

func encode(k store.CacheKey, lim ext.Limits) ([]byte, error) {
	return json.Marshal(EntryFor(k, lim))
}

// SyncKey upserts one key's entry. A missing row returns store.ErrNotFound.
func (w *Writer) SyncKey(ctx context.Context, keyID string) error {
	k, err := store.GetIngestKeyForCache(ctx, w.DB.SQL, w.DB.Dialect, keyID)
	if err != nil {
		return err
	}
	lim, err := w.Limits.Limits(ctx, k.TenantID)
	if err != nil {
		return fmt.Errorf("keysync: limits for %s: %w", k.TenantID, err)
	}
	b, err := encode(k, lim)
	if err != nil {
		return err
	}
	if _, err := w.KV.Put(ctx, k.Hash, b); err != nil {
		return fmt.Errorf("keysync: put %s: %w", k.KeyID, err)
	}
	return nil
}

type kvItem struct {
	val []byte
	rev uint64
}

// snapshot returns every live entry of the bucket with its revision. It is
// taken before the database is read, so anything written later is never
// mistaken for an orphan, and its revisions let writes be compare-and-swap.
func (w *Writer) snapshot(ctx context.Context) (map[string]kvItem, error) {
	wt, err := w.KV.WatchAll(ctx, jetstream.IgnoreDeletes())
	if err != nil {
		return nil, fmt.Errorf("keysync: snapshot: %w", err)
	}
	defer wt.Stop()
	out := map[string]kvItem{}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e, ok := <-wt.Updates():
			if !ok {
				return nil, errors.New("keysync: snapshot: watch closed")
			}
			if e == nil {
				return out, nil
			}
			out[e.Key()] = kvItem{val: e.Value(), rev: e.Revision()}
		}
	}
}

// deleteIf removes hash only if it is still at rev; a conflict means a fresher
// writer got there first, so it reports false without error.
func (w *Writer) deleteIf(ctx context.Context, hash string, rev uint64) (bool, error) {
	err := w.KV.Delete(ctx, hash, jetstream.LastRevision(rev))
	if err == nil {
		return true, nil
	}
	if queue.IsWrongLastSequence(err) {
		return false, nil
	}
	return false, fmt.Errorf("keysync: delete %s: %w", hash, err)
}

// SyncTenant upserts the tenant's keys and deletes KV entries of that tenant
// that no longer have a row.
func (w *Writer) SyncTenant(ctx context.Context, tenantID string) error {
	snap, err := w.snapshot(ctx)
	if err != nil {
		return err
	}
	keys, err := store.ListIngestKeysForCacheByTenant(ctx, w.DB.SQL, w.DB.Dialect, tenantID)
	if err != nil {
		return err
	}
	desired := make(map[string]struct{}, len(keys))
	if len(keys) > 0 {
		lim, err := w.Limits.Limits(ctx, tenantID)
		if err != nil {
			return fmt.Errorf("keysync: limits for %s: %w", tenantID, err)
		}
		for _, k := range keys {
			b, err := encode(k, lim)
			if err != nil {
				return err
			}
			desired[k.Hash] = struct{}{}
			if _, err := w.KV.Put(ctx, k.Hash, b); err != nil {
				return fmt.Errorf("keysync: put %s: %w", k.KeyID, err)
			}
		}
	}
	for hash, it := range snap {
		if _, ok := desired[hash]; ok {
			continue
		}
		var ent ingest.Entry
		if json.Unmarshal(it.val, &ent) == nil && ent.TenantID != tenantID {
			continue
		}
		// Unparseable entries are garbage too: the cache drops them anyway.
		if _, err := w.deleteIf(ctx, hash, it.rev); err != nil {
			return err
		}
	}
	return nil
}

// Reconcile makes KV match the database. Entries are written by compare-and-
// swap against the revision seen in a snapshot taken before the database read,
// so a concurrent SyncKey (fresher state) always wins and a stale row can never
// resurrect a revoked key; conflicting keys are skipped and re-checked next
// run. Only keys in that snapshot can be deleted as orphans. A tenant whose
// limits cannot be fetched keeps its current entries and is reported in the
// returned error.
func (w *Writer) Reconcile(ctx context.Context) (Stats, error) {
	var st Stats
	snap, err := w.snapshot(ctx)
	if err != nil {
		return st, err
	}
	if w.afterSnapshot != nil {
		w.afterSnapshot()
	}
	keys, err := store.ListIngestKeysForCache(ctx, w.DB.SQL, w.DB.Dialect)
	if err != nil {
		return st, err
	}
	if w.afterDBRead != nil {
		w.afterDBRead()
	}
	limits := map[string]ext.Limits{}
	failed := map[string]error{}
	desired := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		desired[k.Hash] = struct{}{}
		if failed[k.TenantID] != nil {
			continue
		}
		lim, ok := limits[k.TenantID]
		if !ok {
			lim, err = w.Limits.Limits(ctx, k.TenantID)
			if err != nil {
				failed[k.TenantID] = fmt.Errorf("keysync: limits for %s: %w", k.TenantID, err)
				continue
			}
			limits[k.TenantID] = lim
		}
		b, err := encode(k, lim)
		if err != nil {
			return st, err
		}
		cur, exists := snap[k.Hash]
		if exists && bytes.Equal(cur.val, b) {
			continue
		}
		if exists {
			_, err = w.KV.Update(ctx, k.Hash, b, cur.rev)
		} else {
			_, err = w.KV.Create(ctx, k.Hash, b)
		}
		if queue.IsWrongLastSequence(err) {
			continue
		}
		if err != nil {
			return st, fmt.Errorf("keysync: write %s: %w", k.KeyID, err)
		}
		st.Put++
	}
	for hash, it := range snap {
		if _, ok := desired[hash]; ok {
			continue
		}
		done, err := w.deleteIf(ctx, hash, it.rev)
		if err != nil {
			return st, err
		}
		if done {
			st.Deleted++
		}
	}
	errs := make([]error, 0, len(failed))
	for _, e := range failed {
		errs = append(errs, e)
	}
	return st, errors.Join(errs...)
}

// Run reconciles once immediately, then every interval, until ctx is done.
func (w *Writer) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		st, err := w.Reconcile(ctx)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			w.log().Warn("keysync: reconcile", "err", err)
		case st != (Stats{}):
			w.log().Info("keysync: reconciled", "put", st.Put, "deleted", st.Deleted)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
