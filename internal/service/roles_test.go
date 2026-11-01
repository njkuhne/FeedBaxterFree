package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

var seq atomic.Int64

func next(prefix string) string { return fmt.Sprintf("%s%d", prefix, seq.Add(1)) }

// call is one capability row: it prepares its own target, then calls the method as p.
type call struct {
	name      string
	adminOnly bool
	do        func(t *testing.T, f *fixture, p ext.Principal) error
}

func capabilityRows() []call {
	ctx := context.Background()
	someFeedback := func(t *testing.T, f *fixture) []string {
		return f.seed(t, f.tenant.ID, f.channel.ID, next("m"))
	}
	return []call{
		{"list feedback", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.ListFeedback(ctx, p, Filter{}, "", 10)
			return err
		}},
		{"get feedback", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.GetFeedback(ctx, p, someFeedback(t, f)[0])
			return err
		}},
		{"list channels", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.ListChannels(ctx, p)
			return err
		}},
		{"list tags", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.ListTags(ctx, p)
			return err
		}},
		{"stats", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.Stats(ctx, p, Filter{})
			return err
		}},
		{"set status", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.SetStatus(ctx, p, someFeedback(t, f), "reviewed")
			return err
		}},
		{"add tags", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.AddTags(ctx, p, someFeedback(t, f), []string{"x"})
			return err
		}},
		{"remove tags", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.RemoveTags(ctx, p, someFeedback(t, f), []string{"x"})
			return err
		}},
		{"export", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			return f.svc.Export(ctx, p, Filter{}, func(store.Feedback) error { return nil })
		}},
		{"me", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.Me(ctx, p)
			return err
		}},
		{"own tokens", false, func(t *testing.T, f *fixture, p ext.Principal) error {
			tk, _, err := f.svc.CreateToken(ctx, p, "t")
			if err != nil {
				return err
			}
			if _, err := f.svc.ListTokens(ctx, p); err != nil {
				return err
			}
			return f.svc.RevokeToken(ctx, p, tk.ID)
		}},
		{"delete feedback", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			return f.svc.DeleteFeedback(ctx, p, someFeedback(t, f)[0])
		}},
		{"delete user feedback", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.DeleteUserFeedback(ctx, p, next("end-user"))
			return err
		}},
		{"create channel", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.CreateChannel(ctx, p, next("ch"))
			return err
		}},
		{"rename channel", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			c := f.newChannel(t, f.tenant.ID, next("ch"))
			return f.svc.RenameChannel(ctx, p, c.ID, next("renamed"))
		}},
		{"delete channel", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			c := f.newChannel(t, f.tenant.ID, next("ch"))
			return f.svc.DeleteChannel(ctx, p, c.ID)
		}},
		{"list keys", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.ListKeys(ctx, p, "")
			return err
		}},
		{"create key", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, _, err := f.svc.CreateKey(ctx, p, f.channel.ID)
			return err
		}},
		{"revoke key", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			k, err := store.CreateIngestKey(ctx, f.db.SQL, f.db.Dialect, f.tenant.ID, f.channel.ID, store.GenerateIngestKey(), f.ms())
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.svc.RevokeKey(ctx, p, k.ID)
			return err
		}},
		{"list users", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.ListUsers(ctx, p)
			return err
		}},
		{"create user", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, _, err := f.svc.CreateUser(ctx, p, next("u")+"@acme.test", "reviewer")
			return err
		}},
		{"reset password", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			u := f.newUser(t, f.tenant.ID, next("u")+"@acme.test", "reviewer")
			_, err := f.svc.ResetPassword(ctx, p, u.ID)
			return err
		}},
		{"set role", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			u := f.newUser(t, f.tenant.ID, next("u")+"@acme.test", "reviewer")
			return f.svc.SetRole(ctx, p, u.ID, "admin")
		}},
		{"delete user", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			u := f.newUser(t, f.tenant.ID, next("u")+"@acme.test", "reviewer")
			return f.svc.DeleteUser(ctx, p, u.ID)
		}},
		{"ops", true, func(t *testing.T, f *fixture, p ext.Principal) error {
			_, err := f.svc.Ops(ctx, p)
			return err
		}},
	}
}

func TestRoleMatrix(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		f := newFixture(t, db)
		f.svc.Now = time.Now
		for _, row := range capabilityRows() {
			t.Run(row.name, func(t *testing.T) {
				if err := row.do(t, f, f.adminP); err != nil {
					t.Fatalf("admin: %v", err)
				}
				err := row.do(t, f, f.revP)
				if row.adminOnly {
					wantKind(t, err, KindForbidden)
				} else if err != nil {
					t.Fatalf("reviewer: %v", err)
				}
			})
		}
	})
}

func TestUnauthenticatedPrincipal(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		bad := []ext.Principal{
			{},
			{UserID: f.admin.ID, TenantID: f.tenant.ID},                           // no role
			{UserID: f.admin.ID, TenantID: f.tenant.ID, Role: "owner", Via: "ui"}, // unknown role
			{UserID: f.admin.ID, Role: ext.RoleAdmin, Via: "ui"},                  // no tenant
			{TenantID: f.tenant.ID, Role: ext.RoleAdmin, Via: "ui"},               // no user
		}
		now := time.Now()
		for i, p := range bad {
			calls := map[string]error{}
			rec := func(name string, err error) { calls[name] = err }
			_, err := s.ListFeedback(ctx, p, Filter{}, "", 1)
			rec("ListFeedback", err)
			_, err = s.GetFeedback(ctx, p, "x")
			rec("GetFeedback", err)
			_, err = s.SetStatus(ctx, p, []string{"x"}, "new")
			rec("SetStatus", err)
			_, err = s.AddTags(ctx, p, []string{"x"}, []string{"t"})
			rec("AddTags", err)
			_, err = s.RemoveTags(ctx, p, []string{"x"}, []string{"t"})
			rec("RemoveTags", err)
			_, err = s.ListTags(ctx, p)
			rec("ListTags", err)
			_, err = s.Stats(ctx, p, Filter{From: &now, To: &now})
			rec("Stats", err)
			rec("DeleteFeedback", s.DeleteFeedback(ctx, p, "x"))
			_, err = s.DeleteUserFeedback(ctx, p, "x")
			rec("DeleteUserFeedback", err)
			rec("Export", s.Export(ctx, p, Filter{}, func(store.Feedback) error { return nil }))
			_, err = s.ListChannels(ctx, p)
			rec("ListChannels", err)
			_, err = s.CreateChannel(ctx, p, "x")
			rec("CreateChannel", err)
			rec("RenameChannel", s.RenameChannel(ctx, p, "x", "y"))
			rec("DeleteChannel", s.DeleteChannel(ctx, p, "x"))
			_, err = s.ListKeys(ctx, p, "")
			rec("ListKeys", err)
			_, _, err = s.CreateKey(ctx, p, "x")
			rec("CreateKey", err)
			_, err = s.RevokeKey(ctx, p, "x")
			rec("RevokeKey", err)
			_, err = s.Me(ctx, p)
			rec("Me", err)
			_, err = s.ListUsers(ctx, p)
			rec("ListUsers", err)
			_, _, err = s.CreateUser(ctx, p, "a@b.test", "admin")
			rec("CreateUser", err)
			_, err = s.ResetPassword(ctx, p, "x")
			rec("ResetPassword", err)
			rec("SetRole", s.SetRole(ctx, p, "x", "admin"))
			rec("DeleteUser", s.DeleteUser(ctx, p, "x"))
			rec("ChangePassword", s.ChangePassword(ctx, p, "a", "b", ""))
			_, _, err = s.CreateToken(ctx, p, "x")
			rec("CreateToken", err)
			_, err = s.ListTokens(ctx, p)
			rec("ListTokens", err)
			rec("RevokeToken", s.RevokeToken(ctx, p, "x"))
			_, err = s.Ops(ctx, p)
			rec("Ops", err)
			if len(calls) != 28 {
				t.Fatalf("principal %d: %d methods exercised", i, len(calls))
			}
			for name, err := range calls {
				if KindOf(err) != KindUnauthenticated {
					t.Errorf("principal %d: %s -> %v", i, name, err)
				}
			}
		}
	})
}

func TestTenantIsolation(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		tb := f.newTenant(t, "Other")
		chB := f.newChannel(t, tb.ID, "web")
		adminB := principal(f.newUser(t, tb.ID, "admin@other.test", "admin"), "ui")
		_ = chB

		ids := f.seed(t, f.tenant.ID, f.channel.ID, "secret")
		id := ids[0]
		_, err := store.GetFeedback(ctx, db.SQL, db.Dialect, f.tenant.ID, id)
		mustNil(t, err)
		k, plain, err := s.CreateKey(ctx, f.adminP, f.channel.ID)
		mustNil(t, err)
		_, tok, err := s.CreateToken(ctx, f.adminP, "mine")
		mustNil(t, err)
		_ = tok

		_, err = s.GetFeedback(ctx, adminB, id)
		wantKind(t, err, KindNotFound)
		pg, err := s.ListFeedback(ctx, adminB, Filter{}, "", 10)
		mustNil(t, err)
		if len(pg.Items) != 0 {
			t.Fatal("tenant B lists A's feedback")
		}
		res, err := s.SetStatus(ctx, adminB, []string{id}, "ignored")
		mustNil(t, err)
		if res.Updated != 0 || len(res.NotFound) != 1 {
			t.Fatalf("set status %+v", res)
		}
		res, err = s.AddTags(ctx, adminB, []string{id}, []string{"stolen"})
		mustNil(t, err)
		if res.Updated != 0 || len(res.NotFound) != 1 {
			t.Fatalf("add tags %+v", res)
		}
		res, err = s.RemoveTags(ctx, adminB, []string{id}, []string{"stolen"})
		mustNil(t, err)
		if res.Updated != 0 {
			t.Fatalf("remove tags %+v", res)
		}
		wantKind(t, s.DeleteFeedback(ctx, adminB, id), KindNotFound)
		n, err := s.DeleteUserFeedback(ctx, adminB, "anyone")
		mustNil(t, err)
		if n != 0 {
			t.Fatal("deleted something")
		}
		var seen int
		mustNil(t, s.Export(ctx, adminB, Filter{}, func(store.Feedback) error { seen++; return nil }))
		if seen != 0 {
			t.Fatal("export leaked")
		}
		st, _ := s.Stats(ctx, adminB, Filter{})
		if st.Total != 0 {
			t.Fatal("stats leaked")
		}

		_, err = s.RevokeKey(ctx, adminB, k.ID)
		wantKind(t, err, KindNotFound)
		_, _, err = s.CreateKey(ctx, adminB, f.channel.ID)
		wantKind(t, err, KindNotFound)
		_, err = s.ListKeys(ctx, adminB, f.channel.ID)
		wantKind(t, err, KindNotFound)
		wantKind(t, s.RenameChannel(ctx, adminB, f.channel.ID, "mine"), KindNotFound)
		wantKind(t, s.DeleteChannel(ctx, adminB, f.channel.ID), KindNotFound)
		wantKind(t, s.DeleteUser(ctx, adminB, f.reviewer.ID), KindNotFound)
		wantKind(t, s.SetRole(ctx, adminB, f.reviewer.ID, "admin"), KindNotFound)
		_, err = s.ResetPassword(ctx, adminB, f.reviewer.ID)
		wantKind(t, err, KindNotFound)
		us, _ := s.ListUsers(ctx, adminB)
		if len(us) != 1 {
			t.Fatalf("B sees %d users", len(us))
		}

		// Nothing of A's changed.
		d, err := s.GetFeedback(ctx, f.adminP, id)
		mustNil(t, err)
		if d.Status != "new" || len(d.Tags) != 0 {
			t.Fatalf("A's feedback changed: %+v", d.Feedback)
		}
		if en, ok := f.cacheEntry(t, plain); !ok || en.Revoked {
			t.Fatal("A's key was revoked")
		}
	})
}
