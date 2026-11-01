package store_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func TestGenerateIngestKey(t *testing.T) {
	re := regexp.MustCompile(`^fbx_[a-z2-7]{32}$`)
	hexRe := regexp.MustCompile(`^[0-9a-f]{64}$`)
	seen := map[string]bool{}
	for range 1000 {
		k := store.GenerateIngestKey()
		if len(k) != 36 || !re.MatchString(k) {
			t.Fatalf("bad key %q", k)
		}
		if seen[k] {
			t.Fatalf("duplicate key %q", k)
		}
		seen[k] = true
		h := store.HashIngestKey(k)
		sum := sha256.Sum256([]byte(k))
		if !hexRe.MatchString(h) || h != hex.EncodeToString(sum[:]) {
			t.Fatalf("bad hash %q", h)
		}
		if p := store.DisplayPrefix(k); len(p) != 12 || p != k[:12] {
			t.Fatalf("bad prefix %q", p)
		}
	}
}

func TestCreateIngestKeyStoresHashOnly(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		plain := store.GenerateIngestKey()
		k, err := store.CreateIngestKey(ctx, db.SQL, db.Dialect, tn.ID, ch.ID, plain, 500)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(k.ID, "ky_") {
			t.Errorf("id = %q", k.ID)
		}
		var id, tenant, channel, prefix, hash string
		var created int64
		var revoked *int64
		err = db.SQL.QueryRow(db.Dialect.Rebind(
			"SELECT id, tenant_id, channel_id, prefix, hash, created_at, revoked_at FROM ingest_keys WHERE id = ?"), k.ID).
			Scan(&id, &tenant, &channel, &prefix, &hash, &created, &revoked)
		if err != nil {
			t.Fatal(err)
		}
		if hash != store.HashIngestKey(plain) || prefix != store.DisplayPrefix(plain) || revoked != nil || created != 500 {
			t.Errorf("row = %q %q %d %v", prefix, hash, created, revoked)
		}
		for _, v := range []string{id, tenant, channel, prefix, hash} {
			if strings.Contains(v, plain) {
				t.Errorf("column contains plaintext: %q", v)
			}
		}
	})
}

func TestListIngestKeysForCache(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		d := db.Dialect
		t1, err := store.CreateTenant(ctx, db.SQL, d, "one", 1, 10)
		if err != nil {
			t.Fatal(err)
		}
		t2, err := store.CreateTenant(ctx, db.SQL, d, "two", 15, 20)
		if err != nil {
			t.Fatal(err)
		}
		c1, _ := store.CreateChannel(ctx, db.SQL, d, t1.ID, "a", 1)
		c2, _ := store.CreateChannel(ctx, db.SQL, d, t2.ID, "b", 1)

		type spec struct {
			tn store.Tenant
			ch store.Channel
		}
		var keys []store.IngestKey
		for _, s := range []spec{{t1, c1}, {t1, c1}, {t2, c2}} {
			k, err := store.CreateIngestKey(ctx, db.SQL, d, s.tn.ID, s.ch.ID, store.GenerateIngestKey(), 30)
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, k)
		}
		if err := store.RevokeIngestKey(ctx, db.SQL, d, keys[1].ID, 111); err != nil {
			t.Fatal(err)
		}
		if err := store.RevokeIngestKey(ctx, db.SQL, d, keys[1].ID, 222); err != nil {
			t.Fatal(err)
		}
		var at int64
		if err := db.SQL.QueryRow(d.Rebind("SELECT revoked_at FROM ingest_keys WHERE id = ?"), keys[1].ID).Scan(&at); err != nil || at != 111 {
			t.Errorf("revoked_at = %d err=%v", at, err)
		}
		if err := store.RevokeIngestKey(ctx, db.SQL, d, "ky_nope", 1); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("revoke unknown err = %v", err)
		}

		want := map[string]store.CacheKey{}
		for i, k := range keys {
			tn, ch, bms := t1.ID, c1.ID, 1
			if i == 2 {
				tn, ch, bms = t2.ID, c2.ID, 15
			}
			want[k.ID] = store.CacheKey{KeyID: k.ID, Hash: k.Hash, TenantID: tn, ChannelID: ch, Revoked: i == 1, BillingMonthStart: bms}
		}
		all, err := store.ListIngestKeysForCache(ctx, db.SQL, d)
		if err != nil || len(all) != 3 {
			t.Fatalf("all = %d err=%v", len(all), err)
		}
		for i, g := range all {
			if g != want[g.KeyID] {
				t.Errorf("got %+v want %+v", g, want[g.KeyID])
			}
			if i > 0 && all[i-1].KeyID >= g.KeyID {
				t.Error("not ordered by id")
			}
		}
		by, err := store.ListIngestKeysForCacheByTenant(ctx, db.SQL, d, t1.ID)
		if err != nil || len(by) != 2 {
			t.Fatalf("by tenant = %d err=%v", len(by), err)
		}
		for _, g := range by {
			if g.TenantID != t1.ID {
				t.Errorf("wrong tenant %+v", g)
			}
		}
		g, err := store.GetIngestKeyForCache(ctx, db.SQL, d, keys[2].ID)
		if err != nil || g != want[keys[2].ID] {
			t.Errorf("get = %+v err=%v", g, err)
		}
		if _, err := store.GetIngestKeyForCache(ctx, db.SQL, d, "ky_nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("get unknown err = %v", err)
		}
	})
}
