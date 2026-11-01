package service

import (
	"context"
	"testing"

	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func TestLastAdmin(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc

		wantKind(t, s.SetRole(ctx, f.adminP, f.admin.ID, "reviewer"), KindConflict)
		wantKind(t, s.DeleteUser(ctx, f.adminP, f.admin.ID), KindConflict)

		second := f.newUser(t, f.tenant.ID, "admin2@acme.test", "admin")
		secondP := principal(second, "ui")
		// Self-delete is refused even with two admins.
		wantKind(t, s.DeleteUser(ctx, f.adminP, f.admin.ID), KindConflict)
		// One can be demoted or removed while the other stays.
		mustNil(t, s.SetRole(ctx, f.adminP, second.ID, "reviewer"))
		wantKind(t, s.SetRole(ctx, f.adminP, f.admin.ID, "reviewer"), KindConflict)
		mustNil(t, s.SetRole(ctx, f.adminP, second.ID, "admin"))
		mustNil(t, s.DeleteUser(ctx, secondP, f.admin.ID))
		wantKind(t, s.DeleteUser(ctx, secondP, "usr_missing"), KindNotFound)
		wantKind(t, s.SetRole(ctx, secondP, f.reviewer.ID, "boss"), KindInvalid)
	})
}

func TestResetPassword(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc

		for _, bad := range []string{"", "no-at", "a@b@c", "@x", "x@", "a b@c"} {
			_, _, err := s.CreateUser(ctx, f.adminP, bad, "reviewer")
			wantKind(t, err, KindInvalid)
		}
		_, _, err := s.CreateUser(ctx, f.adminP, "x@y.test", "owner")
		wantKind(t, err, KindInvalid)
		u, temp, err := s.CreateUser(ctx, f.adminP, "  New@Acme.test ", "reviewer")
		mustNil(t, err)
		if u.Email != "new@acme.test" || !u.MustChangePassword || len(temp) != 20 {
			t.Fatalf("user %+v", u)
		}
		if u.PasswordHash == "" || u.PasswordHash == temp {
			t.Fatal("password not hashed")
		}
		_, _, err = s.CreateUser(ctx, f.adminP, "NEW@acme.test", "admin")
		wantKind(t, err, KindConflict)
		if _, err := s.Login(ctx, f.tenant.ID, "new@acme.test", temp); err != nil {
			t.Fatal(err)
		}

		// Reset: must_change, sessions gone, tokens kept.
		tk, _, _ := s.CreateSession(ctx, f.reviewer)
		_, plain, _ := s.CreateToken(ctx, f.revP, "keep me")
		temp2, err := s.ResetPassword(ctx, f.adminP, f.reviewer.ID)
		mustNil(t, err)
		got, _ := store.GetUser(ctx, db.SQL, db.Dialect, f.tenant.ID, f.reviewer.ID)
		if !got.MustChangePassword || !VerifyPassword(got.PasswordHash, temp2) {
			t.Fatal("reset did not set the temp password and must_change")
		}
		_, err = s.ResolveSession(ctx, tk)
		wantKind(t, err, KindUnauthenticated)
		if _, err := s.PrincipalFromToken(ctx, plain); err != nil {
			t.Fatalf("token should survive reset: %v", err)
		}
		_, err = s.ResetPassword(ctx, f.adminP, f.admin.ID)
		wantKind(t, err, KindInvalid)
		_, err = s.ResetPassword(ctx, f.adminP, "usr_missing")
		wantKind(t, err, KindNotFound)
	})
}
