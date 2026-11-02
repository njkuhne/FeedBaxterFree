package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func TestUserCRUD(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, _ := newTenant(t, db, "acme")
		other, _ := newTenant(t, db, "other")
		a := seedUser(t, db, tn.ID, "a@example.com", "admin", 10)
		if a.ID == "" || a.ID[:3] != "us_" {
			t.Errorf("id = %q", a.ID)
		}
		r := seedUser(t, db, tn.ID, "r@example.com", "reviewer", 20)
		b := seedUser(t, db, tn.ID, "b@example.com", "admin", 30)

		if _, err := store.CreateUser(ctx, db.SQL, d, store.User{TenantID: tn.ID, Email: "a@example.com", PasswordHash: "x", Role: "admin"}); !errors.Is(err, store.ErrConflict) {
			t.Errorf("duplicate email err = %v", err)
		}
		seedUser(t, db, other.ID, "a@example.com", "admin", 5) // same email, other tenant

		got, err := store.GetUser(ctx, db.SQL, d, tn.ID, a.ID)
		if err != nil || got != a {
			t.Errorf("GetUser = %+v, %v", got, err)
		}
		if _, err := store.GetUser(ctx, db.SQL, d, other.ID, a.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant GetUser err = %v", err)
		}
		if got, err := store.GetUserByEmail(ctx, db.SQL, d, tn.ID, "r@example.com"); err != nil || got.ID != r.ID {
			t.Errorf("GetUserByEmail = %+v, %v", got, err)
		}
		if _, err := store.GetUserByEmail(ctx, db.SQL, d, tn.ID, "R@example.com"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("email match must be exact, err = %v", err)
		}
		if fa, err := store.FirstAdmin(ctx, db.SQL, d, tn.ID); err != nil || fa.ID != a.ID {
			t.Errorf("FirstAdmin = %+v, %v", fa, err)
		}
		if n, _ := store.CountAdmins(ctx, db.SQL, d, tn.ID); n != 2 {
			t.Errorf("CountAdmins = %d", n)
		}
		if n, _ := store.CountUsers(ctx, db.SQL, d, tn.ID); n != 3 {
			t.Errorf("CountUsers = %d", n)
		}
		list, err := store.ListUsers(ctx, db.SQL, d, tn.ID)
		if err != nil || len(list) != 3 || list[0].ID != a.ID || list[1].ID != r.ID || list[2].ID != b.ID {
			t.Errorf("ListUsers = %+v, %v", list, err)
		}

		if err := store.SetUserRole(ctx, db.SQL, d, tn.ID, r.ID, "admin", 99); err != nil {
			t.Fatal(err)
		}
		if err := store.SetUserPassword(ctx, db.SQL, d, tn.ID, r.ID, "newhash", true, 100); err != nil {
			t.Fatal(err)
		}
		got, _ = store.GetUser(ctx, db.SQL, d, tn.ID, r.ID)
		if got.Role != "admin" || got.PasswordHash != "newhash" || !got.MustChangePassword || got.UpdatedAt != 100 {
			t.Errorf("after updates: %+v", got)
		}
		if n, _ := store.CountAdmins(ctx, db.SQL, d, tn.ID); n != 3 {
			t.Errorf("CountAdmins = %d", n)
		}
		if err := store.SetUserRole(ctx, db.SQL, d, other.ID, r.ID, "admin", 1); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant SetUserRole err = %v", err)
		}
		if err := store.SetUserPassword(ctx, db.SQL, d, tn.ID, "us_none", "h", false, 1); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("SetUserPassword err = %v", err)
		}
		if err := store.LockAdmins(ctx, db.SQL, d, tn.ID); err != nil {
			t.Errorf("LockAdmins: %v", err)
		}
		if err := store.DeleteUser(ctx, db.SQL, d, tn.ID, b.ID); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteUser(ctx, db.SQL, d, tn.ID, b.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("second delete err = %v", err)
		}
	})
}

func TestDeleteUserCascades(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, _ := newTenant(t, db, "acme")
		u := seedUser(t, db, tn.ID, "a@example.com", "admin", 1)
		if err := store.CreateSession(ctx, db.SQL, d, store.Session{ID: "s1", TenantID: tn.ID, UserID: u.ID, ExpiresAt: 10, CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateToken(ctx, db.SQL, d, store.Token{TenantID: tn.ID, UserID: u.ID, Name: "t", Prefix: "fbp_ab", Hash: "h1", CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteUser(ctx, db.SQL, d, tn.ID, u.ID); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "sessions"); n != 0 {
			t.Errorf("sessions = %d", n)
		}
		if n := count(t, db, "personal_access_tokens"); n != 0 {
			t.Errorf("tokens = %d", n)
		}
	})
}

func TestSessionLifecycle(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, _ := newTenant(t, db, "acme")
		u := seedUser(t, db, tn.ID, "a@example.com", "reviewer", 1)
		if err := store.SetUserPassword(ctx, db.SQL, d, tn.ID, u.ID, "h", true, 2); err != nil {
			t.Fatal(err)
		}
		for i, id := range []string{"s1", "s2", "s3"} {
			if err := store.CreateSession(ctx, db.SQL, d, store.Session{ID: id, TenantID: tn.ID, UserID: u.ID, ExpiresAt: int64(100 * (i + 1)), CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
		}
		s, gu, err := store.GetSessionUser(ctx, db.SQL, d, "s1")
		if err != nil || s.ID != "s1" || s.UserID != u.ID || s.ExpiresAt != 100 || gu.ID != u.ID || gu.Role != "reviewer" || !gu.MustChangePassword {
			t.Fatalf("GetSessionUser = %+v %+v %v", s, gu, err)
		}
		if _, _, err := store.GetSessionUser(ctx, db.SQL, d, "nope"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("missing err = %v", err)
		}
		if err := store.SetSessionExpiry(ctx, db.SQL, d, "s1", 5000); err != nil {
			t.Fatal(err)
		}
		if s, _, _ := store.GetSessionUser(ctx, db.SQL, d, "s1"); s.ExpiresAt != 5000 {
			t.Errorf("expiry = %d", s.ExpiresAt)
		}
		if n, err := store.DeleteExpiredSessions(ctx, db.SQL, d, 250); err != nil || n != 1 { // s2 (200)
			t.Errorf("DeleteExpiredSessions = %d, %v", n, err)
		}
		if err := store.DeleteSession(ctx, db.SQL, d, "s3"); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteSession(ctx, db.SQL, d, "s3"); err != nil {
			t.Errorf("deleting an absent session: %v", err)
		}
		for _, id := range []string{"a", "b", "c"} {
			if err := store.CreateSession(ctx, db.SQL, d, store.Session{ID: id, TenantID: tn.ID, UserID: u.ID, ExpiresAt: 9999, CreatedAt: 1}); err != nil {
				t.Fatal(err)
			}
		}
		if n, err := store.DeleteUserSessions(ctx, db.SQL, d, tn.ID, u.ID, "a"); err != nil || n != 3 { // s1, b, c
			t.Errorf("except = %d, %v", n, err)
		}
		if _, _, err := store.GetSessionUser(ctx, db.SQL, d, "a"); err != nil {
			t.Errorf("excepted session gone: %v", err)
		}
		if n, err := store.DeleteUserSessions(ctx, db.SQL, d, tn.ID, u.ID, ""); err != nil || n != 1 {
			t.Errorf("all = %d, %v", n, err)
		}
	})
}

func TestTokenLifecycle(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, _ := newTenant(t, db, "acme")
		u := seedUser(t, db, tn.ID, "a@example.com", "admin", 1)
		v := seedUser(t, db, tn.ID, "v@example.com", "admin", 1)
		t1, err := store.CreateToken(ctx, db.SQL, d, store.Token{TenantID: tn.ID, UserID: u.ID, Name: "one", Prefix: "fbp_aaaa", Hash: "h1", CreatedAt: 10})
		if err != nil || t1.ID[:3] != "tk_" {
			t.Fatalf("create: %+v %v", t1, err)
		}
		t2, _ := store.CreateToken(ctx, db.SQL, d, store.Token{TenantID: tn.ID, UserID: u.ID, Name: "two", Prefix: "fbp_bbbb", Hash: "h2", CreatedAt: 20})
		list, err := store.ListTokens(ctx, db.SQL, d, tn.ID, u.ID)
		if err != nil || len(list) != 2 || list[0].ID != t2.ID || list[1].ID != t1.ID || list[0].Prefix != "fbp_bbbb" || list[0].LastUsedAt != nil {
			t.Errorf("list = %+v, %v", list, err)
		}
		tok, gu, err := store.GetTokenUser(ctx, db.SQL, d, "h1")
		if err != nil || tok.ID != t1.ID || gu.ID != u.ID {
			t.Errorf("GetTokenUser = %+v %+v %v", tok, gu, err)
		}
		if _, _, err := store.GetTokenUser(ctx, db.SQL, d, "zzz"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
		if err := store.DeleteToken(ctx, db.SQL, d, tn.ID, v.ID, t1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("other user's delete err = %v", err)
		}

		last := func() *int64 {
			l, _ := store.ListTokens(ctx, db.SQL, d, tn.ID, u.ID)
			for _, x := range l {
				if x.ID == t1.ID {
					return x.LastUsedAt
				}
			}
			return nil
		}
		for _, step := range []struct{ now, want int64 }{
			{1_000_000, 1_000_000}, // first use writes
			{1_030_000, 1_000_000}, // within 60 s: no write
			{1_060_000, 1_000_000}, // exactly 60 s: still no write
			{1_060_001, 1_060_001}, // after 60 s: write
		} {
			if err := store.TouchToken(ctx, db.SQL, d, t1.ID, step.now); err != nil {
				t.Fatal(err)
			}
			if got := last(); got == nil || *got != step.want {
				t.Errorf("after touch %d: last_used_at = %v, want %d", step.now, got, step.want)
			}
		}
		if err := store.DeleteToken(ctx, db.SQL, d, tn.ID, u.ID, t1.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestChannelCRUD(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, main := newTenant(t, db, "acme")
		other, _ := newTenant(t, db, "other")
		web, err := store.CreateChannel(ctx, db.SQL, d, tn.ID, "web", 2000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateChannel(ctx, db.SQL, d, tn.ID, "web", 3000); !errors.Is(err, store.ErrConflict) {
			t.Errorf("duplicate create err = %v", err)
		}
		list, err := store.ListChannels(ctx, db.SQL, d, tn.ID)
		if err != nil || len(list) != 2 || list[0].ID != main.ID || list[1].ID != web.ID {
			t.Errorf("list = %+v, %v", list, err)
		}
		if n, _ := store.CountChannels(ctx, db.SQL, d, tn.ID); n != 2 {
			t.Errorf("count = %d", n)
		}
		if err := store.RenameChannel(ctx, db.SQL, d, tn.ID, web.ID, "main"); !errors.Is(err, store.ErrConflict) {
			t.Errorf("rename to existing err = %v", err)
		}
		if err := store.RenameChannel(ctx, db.SQL, d, tn.ID, web.ID, "site"); err != nil {
			t.Fatal(err)
		}
		if err := store.RenameChannel(ctx, db.SQL, d, tn.ID, "ch_none", "x"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("rename missing err = %v", err)
		}
		if got, err := store.GetChannel(ctx, db.SQL, d, tn.ID, web.ID); err != nil || got.Name != "site" {
			t.Errorf("get = %+v, %v", got, err)
		}
		if _, err := store.GetChannel(ctx, db.SQL, d, other.ID, web.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant get err = %v", err)
		}
		if err := store.DeleteChannel(ctx, db.SQL, d, other.ID, web.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant delete err = %v", err)
		}

		seedFB(t, db, tn, web.ID, "fb_1", "hello", nil, 10)
		if _, err := store.CreateIngestKey(ctx, db.SQL, d, tn.ID, web.ID, store.GenerateIngestKey(), 5); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteChannel(ctx, db.SQL, d, tn.ID, web.ID); err != nil {
			t.Fatal(err)
		}
		if n := count(t, db, "ingest_keys"); n != 0 {
			t.Errorf("ingest_keys = %d after channel delete", n)
		}
		fb, err := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_1")
		if err != nil || fb.ChannelName != nil || fb.ChannelID != web.ID {
			t.Errorf("feedback after channel delete: %+v, %v", fb, err)
		}
		if err := store.DeleteChannel(ctx, db.SQL, d, tn.ID, web.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("second delete err = %v", err)
		}
	})
}

func TestRevokeChannelKeys(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		k1, _ := store.CreateIngestKey(ctx, db.SQL, d, tn.ID, ch.ID, store.GenerateIngestKey(), 1)
		k2, _ := store.CreateIngestKey(ctx, db.SQL, d, tn.ID, ch.ID, store.GenerateIngestKey(), 2)
		if err := store.RevokeIngestKey(ctx, db.SQL, d, k1.ID, 50); err != nil {
			t.Fatal(err)
		}
		ids, err := store.RevokeChannelKeys(ctx, db.SQL, d, tn.ID, ch.ID, 100)
		if err != nil || len(ids) != 2 {
			t.Fatalf("ids = %v, %v", ids, err)
		}
		g1, _ := store.GetIngestKey(ctx, db.SQL, d, tn.ID, k1.ID)
		g2, _ := store.GetIngestKey(ctx, db.SQL, d, tn.ID, k2.ID)
		if g1.RevokedAt == nil || *g1.RevokedAt != 50 {
			t.Errorf("already-revoked key changed: %v", g1.RevokedAt)
		}
		if g2.RevokedAt == nil || *g2.RevokedAt != 100 {
			t.Errorf("k2 revoked_at = %v", g2.RevokedAt)
		}
		if ids, err := store.RevokeChannelKeys(ctx, db.SQL, d, tn.ID, "ch_none", 1); err != nil || len(ids) != 0 {
			t.Errorf("unknown channel: %v %v", ids, err)
		}
	})
}

func TestListIngestKeys(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		other, och := newTenant(t, db, "other")
		ch2, _ := store.CreateChannel(ctx, db.SQL, d, tn.ID, "second", 5)
		k1, _ := store.CreateIngestKey(ctx, db.SQL, d, tn.ID, ch.ID, store.GenerateIngestKey(), 10)
		k2, _ := store.CreateIngestKey(ctx, db.SQL, d, tn.ID, ch2.ID, store.GenerateIngestKey(), 20)
		store.CreateIngestKey(ctx, db.SQL, d, other.ID, och.ID, store.GenerateIngestKey(), 30)

		all, err := store.ListIngestKeys(ctx, db.SQL, d, tn.ID, "")
		if err != nil || len(all) != 2 || all[0].ID != k2.ID || all[1].ID != k1.ID {
			t.Errorf("all = %+v, %v", all, err)
		}
		one, err := store.ListIngestKeys(ctx, db.SQL, d, tn.ID, ch.ID)
		if err != nil || len(one) != 1 || one[0].ID != k1.ID || one[0].Prefix != k1.Prefix {
			t.Errorf("one = %+v, %v", one, err)
		}
		if _, err := store.GetIngestKey(ctx, db.SQL, d, other.ID, k1.ID); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant get err = %v", err)
		}
	})
}

func TestDatabaseSize(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		n, err := store.DatabaseSize(context.Background(), db)
		if err != nil || n <= 0 {
			t.Errorf("size = %d, %v", n, err)
		}
	})
}

func TestListChannelsLastReceivedAt(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, main := newTenant(t, db, "acme")
		other, otherMain := newTenant(t, db, "other")
		empty, err := store.CreateChannel(ctx, db.SQL, d, tn.ID, "empty", 2000)
		if err != nil {
			t.Fatal(err)
		}
		created, err := store.CreateChannel(ctx, db.SQL, d, tn.ID, "new", 3000)
		if err != nil || created.LastReceivedAt != nil {
			t.Fatalf("create = %+v, %v", created, err)
		}
		seedFB(t, db, tn, main.ID, "fb_1", "old", nil, 100)
		seedFB(t, db, tn, main.ID, "fb_2", "newest", nil, 900)
		seedFB(t, db, tn, main.ID, "fb_3", "middle", nil, 500)
		seedFB(t, db, other, otherMain.ID, "fb_4", "elsewhere", nil, 5000)
		list, err := store.ListChannels(ctx, db.SQL, d, tn.ID)
		if err != nil || len(list) != 3 {
			t.Fatalf("list = %+v, %v", list, err)
		}
		if got := list[0]; got.ID != main.ID || got.LastReceivedAt == nil || *got.LastReceivedAt != 900 {
			t.Errorf("main = %+v", got)
		}
		if got := list[1]; got.ID != empty.ID || got.LastReceivedAt != nil {
			t.Errorf("empty = %+v", got)
		}
		if list[2].LastReceivedAt != nil {
			t.Errorf("new = %+v", list[2])
		}
		olist, err := store.ListChannels(ctx, db.SQL, d, other.ID)
		if err != nil || len(olist) != 1 || olist[0].LastReceivedAt == nil || *olist[0].LastReceivedAt != 5000 {
			t.Errorf("other tenant list = %+v, %v", olist, err)
		}
	})
}
