package keysync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/ingest"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

const waitTimeout = 5 * time.Second

type fakeLimits struct {
	mu sync.Mutex
	m  map[string]ext.Limits
}

func (f *fakeLimits) set(tenant string, l ext.Limits) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.m[tenant] = l
}

func (f *fakeLimits) Limits(_ context.Context, tenant string) (ext.Limits, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.m[tenant], nil
}

type fixture struct {
	w      *Writer
	lim    *fakeLimits
	o      *queue.Objects
	tenant store.Tenant
	chanID string
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	_, o := queuetest.Embedded(t, queue.Limits{})
	tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, "Acme", 15, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, "web", 1000)
	if err != nil {
		t.Fatal(err)
	}
	lim := &fakeLimits{m: map[string]ext.Limits{tn.ID: {Daily: 10, Monthly: 100}}}
	return &fixture{
		w:   &Writer{DB: db, KV: o.Keys, Limits: lim, Log: slog.New(slog.NewTextHandler(io.Discard, nil))},
		lim: lim, o: o, tenant: tn, chanID: ch.ID,
	}
}

func (f *fixture) newKey(t *testing.T, db *store.DB) (plaintext string, k store.IngestKey) {
	t.Helper()
	plaintext = store.GenerateIngestKey()
	k, err := store.CreateIngestKey(context.Background(), db.SQL, db.Dialect, f.tenant.ID, f.chanID, plaintext, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return plaintext, k
}

func (f *fixture) kvValue(t *testing.T, hash string) (string, bool) {
	t.Helper()
	e, err := f.o.Keys.Get(context.Background(), hash)
	if err != nil {
		return "", false
	}
	return string(e.Value()), true
}

func TestHashMatchesStore(t *testing.T) {
	for _, k := range []string{"", "fbx_abc", store.GenerateIngestKey()} {
		if ingest.HashKey(k) != store.HashIngestKey(k) {
			t.Fatalf("hash mismatch for %q", k)
		}
	}
}

func TestSyncKey(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, k := f.newKey(t, db)
		if err := f.w.SyncKey(ctx, k.ID); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf(`{"key_id":%q,"tenant_id":%q,"channel_id":%q,"daily_limit":10,"monthly_limit":100,"revoked":false,"billing_month_start":15}`,
			k.ID, f.tenant.ID, f.chanID)
		if got, _ := f.kvValue(t, k.Hash); got != want {
			t.Fatalf("entry\n got %s\nwant %s", got, want)
		}
		if err := store.RevokeIngestKey(ctx, db.SQL, db.Dialect, k.ID, 2000); err != nil {
			t.Fatal(err)
		}
		if err := f.w.SyncKey(ctx, k.ID); err != nil {
			t.Fatal(err)
		}
		var e ingest.Entry
		got, _ := f.kvValue(t, k.Hash)
		if err := json.Unmarshal([]byte(got), &e); err != nil || !e.Revoked {
			t.Fatalf("after revoke: %s %v", got, err)
		}
		if err := f.w.SyncKey(ctx, "key_missing"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing row: %v", err)
		}
	})
}

func TestReconcile(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, k1 := f.newKey(t, db)
		_, k2 := f.newKey(t, db)
		_, k3 := f.newKey(t, db)
		for _, k := range []store.IngestKey{k1, k2} {
			if err := f.w.SyncKey(ctx, k.ID); err != nil {
				t.Fatal(err)
			}
		}
		// k1 stale, k3 missing, plus an orphan.
		if _, err := f.o.Keys.Put(ctx, k1.Hash, []byte(`{"key_id":"stale"}`)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.o.Keys.Put(ctx, ingest.HashKey("orphan"), []byte(`{"key_id":"orphan"}`)); err != nil {
			t.Fatal(err)
		}
		st, err := f.w.Reconcile(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st != (Stats{Put: 2, Deleted: 1}) {
			t.Fatalf("stats %+v", st)
		}
		for _, k := range []store.IngestKey{k1, k2, k3} {
			var e ingest.Entry
			v, ok := f.kvValue(t, k.Hash)
			if !ok || json.Unmarshal([]byte(v), &e) != nil || e.KeyID != k.ID {
				t.Fatalf("%s: %q %v", k.ID, v, ok)
			}
		}
		if _, ok := f.kvValue(t, ingest.HashKey("orphan")); ok {
			t.Fatal("orphan still present")
		}
		st, err = f.w.Reconcile(ctx)
		if err != nil || st != (Stats{}) {
			t.Fatalf("second run: %+v %v", st, err)
		}
	})
}

func TestSyncTenantDeletesOrphans(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, k := f.newKey(t, db)
		orphan := ingest.HashKey("gone")
		other := ingest.HashKey("other-tenant")
		put := func(hash string, e ingest.Entry) {
			b, _ := json.Marshal(e)
			if _, err := f.o.Keys.Put(ctx, hash, b); err != nil {
				t.Fatal(err)
			}
		}
		put(orphan, ingest.Entry{KeyID: "key_gone", TenantID: f.tenant.ID})
		put(other, ingest.Entry{KeyID: "key_other", TenantID: "tn_other"})
		if err := f.w.SyncTenant(ctx, f.tenant.ID); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.kvValue(t, k.Hash); !ok {
			t.Fatal("tenant key not written")
		}
		if _, ok := f.kvValue(t, orphan); ok {
			t.Fatal("orphan kept")
		}
		if _, ok := f.kvValue(t, other); !ok {
			t.Fatal("other tenant's entry deleted")
		}
	})
}

func TestCacheSeesWriterChanges(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		cache := ingest.NewKeyCache(slog.New(slog.NewTextHandler(io.Discard, nil)))
		cctx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() { defer wg.Done(); _ = cache.Run(cctx, f.o.Keys) }()
		t.Cleanup(func() { cancel(); wg.Wait() })
		select {
		case <-cache.Ready():
		case <-time.After(waitTimeout):
			t.Fatal("not ready")
		}
		var plaintext string
		plaintext, k := f.newKey(t, db)
		wait := func(what string, cond func(ingest.Entry, bool) bool) {
			t.Helper()
			deadline := time.Now().Add(waitTimeout)
			for {
				e, ok := cache.Lookup(ingest.HashKey(plaintext))
				if cond(e, ok) {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("timed out: %s", what)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		if err := f.w.SyncKey(ctx, k.ID); err != nil {
			t.Fatal(err)
		}
		wait("create", func(e ingest.Entry, ok bool) bool { return ok && e.KeyID == k.ID && !e.Revoked && e.DailyLimit == 10 })

		f.lim.set(f.tenant.ID, ext.Limits{Daily: 20, Monthly: 100})
		if err := f.w.SyncTenant(ctx, f.tenant.ID); err != nil {
			t.Fatal(err)
		}
		wait("limit change", func(e ingest.Entry, ok bool) bool { return ok && e.DailyLimit == 20 })

		if err := store.RevokeIngestKey(ctx, db.SQL, db.Dialect, k.ID, 2000); err != nil {
			t.Fatal(err)
		}
		if err := f.w.SyncKey(ctx, k.ID); err != nil {
			t.Fatal(err)
		}
		wait("revoke", func(e ingest.Entry, ok bool) bool { return ok && e.Revoked })
	})
}

func TestReconcileDoesNotResurrectRevoked(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		_, k := f.newKey(t, db)
		if err := f.w.SyncKey(ctx, k.ID); err != nil {
			t.Fatal(err)
		}
		// Make the entry stale so Reconcile wants to write the not-revoked row state.
		if _, err := f.o.Keys.Put(ctx, k.Hash, []byte(`{"key_id":"stale"}`)); err != nil {
			t.Fatal(err)
		}
		f.w.afterDBRead = func() {
			if err := store.RevokeIngestKey(ctx, db.SQL, db.Dialect, k.ID, 2000); err != nil {
				t.Error(err)
			}
			if err := f.w.SyncKey(ctx, k.ID); err != nil {
				t.Error(err)
			}
		}
		st, err := f.w.Reconcile(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if st.Put != 0 {
			t.Fatalf("conflicting write counted: %+v", st)
		}
		var e ingest.Entry
		v, _ := f.kvValue(t, k.Hash)
		if err := json.Unmarshal([]byte(v), &e); err != nil || !e.Revoked {
			t.Fatalf("revoked key resurrected: %s", v)
		}
	})
}

func TestReconcileKeepsKeyCreatedDuringRun(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		f.newKey(t, db)
		var fresh store.IngestKey
		f.w.afterDBRead = func() { // created after Reconcile read the DB, so not in its desired set
			_, fresh = f.newKey(t, db)
			if err := f.w.SyncKey(ctx, fresh.ID); err != nil {
				t.Error(err)
			}
		}
		if _, err := f.w.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.kvValue(t, fresh.Hash); !ok {
			t.Fatal("fresh key deleted as orphan")
		}
		f.w.afterDBRead = nil
		if st, err := f.w.Reconcile(ctx); err != nil || st.Deleted != 0 {
			t.Fatalf("next run: %+v %v", st, err)
		}
	})
}
