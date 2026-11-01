package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/ingest"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func (f *fixture) cacheEntry(t *testing.T, plaintext string) (ingest.Entry, bool) {
	t.Helper()
	e, err := f.obj.Keys.Get(context.Background(), store.HashIngestKey(plaintext))
	if err != nil {
		return ingest.Entry{}, false
	}
	var en ingest.Entry
	if err := json.Unmarshal(e.Value(), &en); err != nil {
		t.Fatal(err)
	}
	return en, true
}

func TestChannelsCRUD(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		_, err := s.CreateChannel(ctx, f.adminP, " ")
		wantKind(t, err, KindInvalid)
		_, err = s.CreateChannel(ctx, f.adminP, strings.Repeat("x", 65))
		wantKind(t, err, KindInvalid)
		ch, err := s.CreateChannel(ctx, f.adminP, " ios ")
		mustNil(t, err)
		if ch.Name != "ios" {
			t.Fatalf("name %q", ch.Name)
		}
		_, err = s.CreateChannel(ctx, f.adminP, "ios")
		wantKind(t, err, KindConflict)
		wantKind(t, s.RenameChannel(ctx, f.adminP, ch.ID, "web"), KindConflict)
		wantKind(t, s.RenameChannel(ctx, f.adminP, "ch_missing", "z"), KindNotFound)
		mustNil(t, s.RenameChannel(ctx, f.adminP, ch.ID, "iOS app"))
		cs, err := s.ListChannels(ctx, f.revP)
		mustNil(t, err)
		if len(cs) != 2 || (cs[0].Name != "iOS app" && cs[1].Name != "iOS app") {
			t.Fatalf("channels %+v", cs)
		}
	})
}

func TestKeyCreateReachesCache(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		_, _, err := s.CreateKey(ctx, f.adminP, "ch_missing")
		wantKind(t, err, KindNotFound)

		k, plain, err := s.CreateKey(ctx, f.adminP, f.channel.ID)
		mustNil(t, err)
		if !strings.HasPrefix(plain, "fbx_") || k.Hash != store.HashIngestKey(plain) || k.Prefix != store.DisplayPrefix(plain) {
			t.Fatalf("key %+v", k)
		}
		en, ok := f.cacheEntry(t, plain)
		if !ok || en.Revoked || en.TenantID != f.tenant.ID || en.ChannelID != f.channel.ID {
			t.Fatalf("cache entry %+v ok=%v", en, ok)
		}
		keys, err := s.ListKeys(ctx, f.adminP, f.channel.ID)
		mustNil(t, err)
		if len(keys) != 1 {
			t.Fatalf("keys %d", len(keys))
		}
		all, _ := s.ListKeys(ctx, f.adminP, "")
		if len(all) != 1 {
			t.Fatalf("all keys %d", len(all))
		}

		rk, err := s.RevokeKey(ctx, f.adminP, k.ID)
		mustNil(t, err)
		if rk.RevokedAt == nil {
			t.Fatal("RevokedAt not set")
		}
		if en, ok := f.cacheEntry(t, plain); !ok || !en.Revoked {
			t.Fatalf("cache entry after revoke %+v ok=%v", en, ok)
		}
		_, err = s.RevokeKey(ctx, f.adminP, k.ID) // idempotent
		mustNil(t, err)
		_, err = s.RevokeKey(ctx, f.adminP, "key_missing")
		wantKind(t, err, KindNotFound)
	})
}

type recordingKeys struct {
	inner   ext.KeyWriter
	f       *fixture
	t       *testing.T
	revoked map[string]bool // key id -> cache said revoked right after its SyncKey
	tenants int
}

func (r *recordingKeys) SyncKey(ctx context.Context, id string) error {
	if err := r.inner.SyncKey(ctx, id); err != nil {
		return err
	}
	k, err := store.GetIngestKeyForCache(ctx, r.f.db.SQL, r.f.db.Dialect, id)
	if err != nil {
		return err
	}
	e, err := r.f.obj.Keys.Get(ctx, k.Hash)
	if err != nil {
		return err
	}
	var en ingest.Entry
	_ = json.Unmarshal(e.Value(), &en)
	r.revoked[id] = en.Revoked
	return nil
}

func (r *recordingKeys) SyncTenant(ctx context.Context, tenantID string) error {
	r.tenants++
	return r.inner.SyncTenant(ctx, tenantID)
}

func TestDeleteChannel(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		rec := &recordingKeys{inner: s.Keys, f: f, t: t, revoked: map[string]bool{}}
		s.Keys = rec

		// The only channel cannot go.
		wantKind(t, s.DeleteChannel(ctx, f.adminP, f.channel.ID), KindConflict)

		ch2, err := s.CreateChannel(ctx, f.adminP, "ios")
		mustNil(t, err)
		k, plain, err := s.CreateKey(ctx, f.adminP, ch2.ID)
		mustNil(t, err)
		ids := f.seed(t, f.tenant.ID, ch2.ID, "from ios")

		wantKind(t, s.DeleteChannel(ctx, f.adminP, "ch_missing"), KindNotFound)
		mustNil(t, s.DeleteChannel(ctx, f.adminP, ch2.ID))

		if !rec.revoked[k.ID] {
			t.Fatal("key was not revoked in the cache before the channel went")
		}
		if rec.tenants != 1 {
			t.Fatalf("SyncTenant calls = %d", rec.tenants)
		}
		if _, ok := f.cacheEntry(t, plain); ok {
			t.Fatal("orphaned cache entry still present")
		}
		if _, err := store.GetIngestKey(ctx, db.SQL, db.Dialect, f.tenant.ID, k.ID); err == nil {
			t.Fatal("key row survived")
		}
		d, err := s.GetFeedback(ctx, f.adminP, ids[0])
		mustNil(t, err)
		if d.ChannelName != nil || d.ChannelID != ch2.ID {
			t.Fatalf("feedback %+v", d.Feedback)
		}
	})
}
