package service

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := HashPassword("a long enough password")
	mustNil(t, err)
	if !regexp.MustCompile(`^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).MatchString(h) {
		t.Fatalf("not PHC: %q", h)
	}
	if !VerifyPassword(h, "a long enough password") {
		t.Fatal("right password rejected")
	}
	if VerifyPassword(h, "a long enough passworD") {
		t.Fatal("wrong password accepted")
	}
	for _, bad := range []string{"", "plain", "$argon2id$v=19$m=1,t=1$x$y", "$argon2id$v=19$m=65536,t=1,p=4$!!$!!"} {
		if VerifyPassword(bad, "x") {
			t.Fatalf("malformed %q verified", bad)
		}
	}
	// Parameters come from the string: a hash made with other params still verifies.
	salt := []byte("0123456789abcdef")
	key := argonKey("pw", salt, 2, 8*1024, 2, 32)
	phc := "$argon2id$v=19$m=8192,t=2,p=2$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key)
	if !VerifyPassword(phc, "pw") || VerifyPassword(phc, "px") {
		t.Fatal("parameters not parsed from the string")
	}
	// Hashes written by installs before the 19 MiB profile (m=64 MiB, t=1, p=4)
	// must keep verifying.
	key = argonKey("pw", salt, 1, 64*1024, 4, 32)
	phc = "$argon2id$v=19$m=65536,t=1,p=4$" + b64.EncodeToString(salt) + "$" + b64.EncodeToString(key)
	if !VerifyPassword(phc, "pw") || VerifyPassword(phc, "px") {
		t.Fatal("legacy 64 MiB hash does not verify")
	}
	h2, _ := HashPassword("a long enough password")
	if h == h2 {
		t.Fatal("salt not random")
	}
}

func TestTempPassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p := GenerateTempPassword()
		if len(p) != 20 {
			t.Fatalf("len %d", len(p))
		}
		for _, c := range p {
			if !strings.ContainsRune(tempPasswordAlphabet, c) {
				t.Fatalf("char %q outside alphabet", c)
			}
		}
		seen[p] = true
	}
	if len(seen) != 50 {
		t.Fatal("repeated temp passwords")
	}
}

func TestLoginAndSessions(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc

		_, errWrong := s.Login(ctx, f.tenant.ID, "ADMIN@acme.test ", "wrong password!")
		_, errUnknown := s.Login(ctx, f.tenant.ID, "nobody@acme.test", "wrong password!")
		wantKind(t, errWrong, KindUnauthenticated)
		wantKind(t, errUnknown, KindUnauthenticated)
		if errWrong.Error() != errUnknown.Error() {
			t.Fatalf("different messages: %q vs %q", errWrong, errUnknown)
		}
		u, err := s.Login(ctx, f.tenant.ID, "  ADMIN@acme.test ", f.pw)
		mustNil(t, err)
		if u.ID != f.admin.ID {
			t.Fatal("wrong user")
		}
		if _, err := s.Login(ctx, "tn_other", "admin@acme.test", f.pw); KindOf(err) != KindUnauthenticated {
			t.Fatalf("other tenant: %v", err)
		}

		token, exp, err := s.CreateSession(ctx, u)
		mustNil(t, err)
		if len(token) != 43 || !exp.Equal(f.clock.now().Add(SessionTTL)) {
			t.Fatalf("token len %d exp %v", len(token), exp)
		}
		p, err := s.ResolveSession(ctx, token)
		mustNil(t, err)
		if p.UserID != u.ID || p.TenantID != u.TenantID || p.Via != "ui" || p.Role != "admin" || p.MustChangePassword {
			t.Fatalf("principal %+v", p)
		}
		_, err = s.ResolveSession(ctx, "nope")
		wantKind(t, err, KindUnauthenticated)
		_, err = s.ResolveSession(ctx, "")
		wantKind(t, err, KindUnauthenticated)

		expiry := func() int64 {
			sess, _, err := store.GetSessionUser(ctx, db.SQL, db.Dialect, sha256Hex(token))
			mustNil(t, err)
			return sess.ExpiresAt
		}
		first := expiry()
		f.clock.advance(30 * time.Minute)
		_, err = s.ResolveSession(ctx, token)
		mustNil(t, err)
		if expiry() != first {
			t.Fatal("expiry slid within the hour")
		}
		f.clock.advance(31 * time.Minute)
		_, err = s.ResolveSession(ctx, token)
		mustNil(t, err)
		if want := f.clock.now().Add(SessionTTL).UnixMilli(); expiry() != want {
			t.Fatalf("expiry %d, want %d", expiry(), want)
		}

		f.clock.advance(SessionTTL + time.Second)
		_, err = s.ResolveSession(ctx, token)
		wantKind(t, err, KindUnauthenticated)
		if _, _, err := store.GetSessionUser(ctx, db.SQL, db.Dialect, sha256Hex(token)); err == nil {
			t.Fatal("expired session row not deleted")
		}

		// must-change carried; DeleteSession; absent is fine.
		must := f.newUser(t, f.tenant.ID, "must@acme.test", "reviewer")
		mustNil(t, store.SetUserPassword(ctx, db.SQL, db.Dialect, f.tenant.ID, must.ID, f.pwHash, true, f.ms()))
		must, _ = store.GetUser(ctx, db.SQL, db.Dialect, f.tenant.ID, must.ID)
		tk, _, err := s.CreateSession(ctx, must)
		mustNil(t, err)
		p, err = s.ResolveSession(ctx, tk)
		mustNil(t, err)
		if !p.MustChangePassword {
			t.Fatal("must-change flag lost")
		}
		mustNil(t, s.DeleteSession(ctx, tk))
		mustNil(t, s.DeleteSession(ctx, tk))
		_, err = s.ResolveSession(ctx, tk)
		wantKind(t, err, KindUnauthenticated)
	})
}

func TestChangePasswordKeepsCurrentSession(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		mustNil(t, store.SetUserPassword(ctx, db.SQL, db.Dialect, f.tenant.ID, f.admin.ID, f.pwHash, true, f.ms()))
		u, _ := store.GetUser(ctx, db.SQL, db.Dialect, f.tenant.ID, f.admin.ID)
		keep, _, _ := s.CreateSession(ctx, u)
		other, _, _ := s.CreateSession(ctx, u)

		wantKind(t, s.ChangePassword(ctx, f.adminP, "not the password", "brand new password", keep), KindInvalid)
		wantKind(t, s.ChangePassword(ctx, f.adminP, f.pw, "short", keep), KindInvalid)
		wantKind(t, s.ChangePassword(ctx, f.adminP, f.pw, f.pw, keep), KindInvalid)
		mustNil(t, s.ChangePassword(ctx, f.adminP, f.pw, "brand new password", keep))

		if _, err := s.ResolveSession(ctx, keep); err != nil {
			t.Fatalf("current session lost: %v", err)
		}
		_, err := s.ResolveSession(ctx, other)
		wantKind(t, err, KindUnauthenticated)
		u, _ = store.GetUser(ctx, db.SQL, db.Dialect, f.tenant.ID, f.admin.ID)
		if u.MustChangePassword {
			t.Fatal("must_change not cleared")
		}
		if _, err := s.Login(ctx, f.tenant.ID, "admin@acme.test", "brand new password"); err != nil {
			t.Fatal(err)
		}
	})
}

var patRE = regexp.MustCompile(`^fbx_pat_[a-z2-7]{32}$`)

func TestTokens(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc

		_, _, err := s.CreateToken(ctx, f.revP, "   ")
		wantKind(t, err, KindInvalid)
		row, plain, err := s.CreateToken(ctx, f.revP, " laptop ")
		mustNil(t, err)
		if !patRE.MatchString(plain) || row.Name != "laptop" || row.Prefix != plain[:12] || row.Hash == plain {
			t.Fatalf("row %+v plain %q", row, plain)
		}
		p, err := s.PrincipalFromToken(ctx, plain)
		mustNil(t, err)
		if p.Via != "mcp" || p.Role != "reviewer" || p.UserID != f.reviewer.ID || p.TenantID != f.tenant.ID || p.MustChangePassword {
			t.Fatalf("principal %+v", p)
		}
		toks, err := s.ListTokens(ctx, f.revP)
		mustNil(t, err)
		if len(toks) != 1 || toks[0].LastUsedAt == nil {
			t.Fatalf("last_used not touched: %+v", toks)
		}
		if got, _ := s.ListTokens(ctx, f.adminP); len(got) != 0 {
			t.Fatal("admin sees reviewer's tokens")
		}

		for _, bad := range []string{"", "fbx_pat_short", strings.Repeat("a", 40), "fbx_pat_" + strings.Repeat("A", 32), plain + "x"} {
			_, err := s.PrincipalFromToken(ctx, bad)
			wantKind(t, err, KindUnauthenticated)
		}
		_, err = s.PrincipalFromToken(ctx, "fbx_pat_"+strings.Repeat("a", 32))
		wantKind(t, err, KindUnauthenticated)

		// Another user's token cannot be revoked.
		wantKind(t, s.RevokeToken(ctx, f.adminP, row.ID), KindNotFound)
		mustNil(t, s.RevokeToken(ctx, f.revP, row.ID))
		_, err = s.PrincipalFromToken(ctx, plain)
		wantKind(t, err, KindUnauthenticated)

		// Deleting the user kills the token.
		_, plain2, _ := s.CreateToken(ctx, f.revP, "again")
		mustNil(t, s.DeleteUser(ctx, f.adminP, f.reviewer.ID))
		_, err = s.PrincipalFromToken(ctx, plain2)
		wantKind(t, err, KindUnauthenticated)
	})
}
