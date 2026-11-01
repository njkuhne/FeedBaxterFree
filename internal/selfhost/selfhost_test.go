package selfhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/ingest"
	"github.com/njkuhne/feedbaxterfree/internal/ingest/keysync"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

var keyLine = regexp.MustCompile(`ingest key for channel Default: (fbx_[a-z2-7]{32})`)

func TestBootstrapCreatesOnce(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, o := queuetest.Embedded(t, queue.Limits{})
		w := &keysync.Writer{DB: db, KV: o.Keys, Limits: Limits{Daily: 7, Monthly: 70}}
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

		created, err := Bootstrap(ctx, db, w, log, now, "admin@localhost")
		if err != nil || !created {
			t.Fatalf("Bootstrap = %v, %v; want created", created, err)
		}
		m := keyLine.FindStringSubmatch(buf.String())
		if m == nil || len(keyLine.FindAllString(buf.String(), -1)) != 1 {
			t.Fatalf("log = %q; want exactly one key line", buf.String())
		}
		key := m[1]

		tn, err := store.FirstTenant(ctx, db.SQL, db.Dialect)
		if err != nil || tn.Name != TenantName || tn.BillingMonthStart != 1 {
			t.Fatalf("tenant = %+v, %v", tn, err)
		}
		var channel string
		if err := db.SQL.QueryRowContext(ctx, db.Dialect.Rebind(
			"SELECT name FROM channels WHERE tenant_id = ?"), tn.ID).Scan(&channel); err != nil || channel != DefaultChannel {
			t.Fatalf("channel = %q, %v", channel, err)
		}
		keys, err := store.ListIngestKeysForCache(ctx, db.SQL, db.Dialect)
		if err != nil || len(keys) != 1 || keys[0].Hash != store.HashIngestKey(key) {
			t.Fatalf("keys = %+v, %v", keys, err)
		}

		kve, err := o.Keys.Get(ctx, store.HashIngestKey(key))
		if err != nil {
			t.Fatalf("KV entry: %v", err)
		}
		var ent ingest.Entry
		if err := json.Unmarshal(kve.Value(), &ent); err != nil {
			t.Fatal(err)
		}
		if ent.TenantID != tn.ID || ent.KeyID != keys[0].KeyID || ent.DailyLimit != 7 || ent.MonthlyLimit != 70 || ent.Revoked {
			t.Fatalf("KV entry = %+v", ent)
		}

		buf.Reset()
		created, err = Bootstrap(ctx, db, w, log, now, "admin@localhost")
		if err != nil || created || buf.Len() != 0 {
			t.Fatalf("second Bootstrap = %v, %v, log %q; want no-op", created, err, buf.String())
		}
		if n, _ := store.CountTenants(ctx, db.SQL, db.Dialect); n != 1 {
			t.Fatalf("tenants = %d, want 1", n)
		}
	})
}

type failingKeys struct{ ext.KeyWriter }

func (failingKeys) SyncKey(context.Context, string) error { return errors.New("kv down") }

func TestBootstrapSyncFailureIsLogged(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		created, err := Bootstrap(context.Background(), db, failingKeys{}, log, time.Now(), "admin@localhost")
		if err != nil || !created {
			t.Fatalf("Bootstrap = %v, %v; want created despite sync error", created, err)
		}
		if !keyLine.MatchString(buf.String()) || !regexp.MustCompile(`kv down`).MatchString(buf.String()) {
			t.Fatalf("log = %q", buf.String())
		}
	})
}

func TestTenantResolver(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		r := TenantResolver{DB: db}
		if _, err := r.Default(ctx); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Default on empty = %v, want ErrNotFound", err)
		}
		tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, "Acme", 15, 1000)
		if err != nil {
			t.Fatal(err)
		}
		want := ext.Tenant{ID: tn.ID, Name: "Acme", BillingMonthStart: 15}
		if got, err := r.Default(ctx); err != nil || got != want {
			t.Fatalf("Default = %+v, %v; want %+v", got, err, want)
		}
		if got, err := r.ByID(ctx, tn.ID); err != nil || got != want {
			t.Fatalf("ByID = %+v, %v; want %+v", got, err, want)
		}
		if _, err := r.ByID(ctx, "tn_missing"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("ByID missing = %v, want ErrNotFound", err)
		}
	})
}

func TestLimitsFromConfig(t *testing.T) {
	l := LimitsFromConfig(config.Config{DailyLimit: 5, MonthlyLimit: 50})
	for _, id := range []string{"tn_a", "tn_b"} {
		got, err := l.Limits(context.Background(), id)
		if err != nil || got != (ext.Limits{Daily: 5, Monthly: 50}) {
			t.Fatalf("Limits(%s) = %+v, %v", id, got, err)
		}
	}
}

var adminLine = regexp.MustCompile(`admin login admin@localhost temporary password: ([A-Za-z2-9]{20}) `)

func TestBootstrapCreatesAdmin(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		_, o := queuetest.Embedded(t, queue.Limits{})
		w := &keysync.Writer{DB: db, KV: o.Keys, Limits: Limits{}}
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))

		if created, err := Bootstrap(ctx, db, w, log, time.Now(), " Admin@Localhost "); err != nil || !created {
			t.Fatalf("Bootstrap = %v, %v", created, err)
		}
		m := adminLine.FindStringSubmatch(buf.String())
		if m == nil || len(adminLine.FindAllString(buf.String(), -1)) != 1 {
			t.Fatalf("log = %q; want exactly one admin line", buf.String())
		}
		if !strings.Contains(buf.String(), "(you must change it at first login)") {
			t.Fatalf("log = %q", buf.String())
		}
		tn, err := store.FirstTenant(ctx, db.SQL, db.Dialect)
		if err != nil {
			t.Fatal(err)
		}
		svc := &service.Service{DB: db}
		u, err := svc.Login(ctx, tn.ID, "admin@localhost", m[1])
		if err != nil || u.Role != "admin" || !u.MustChangePassword {
			t.Fatalf("Login = %+v, %v", u, err)
		}

		buf.Reset()
		if created, err := Bootstrap(ctx, db, w, log, time.Now(), "admin@localhost"); err != nil || created || buf.Len() != 0 {
			t.Fatalf("second Bootstrap = %v, %v, log %q; want no-op", created, err, buf.String())
		}
		if n, _ := store.CountUsers(ctx, db.SQL, db.Dialect, tn.ID); n != 1 {
			t.Fatalf("users = %d, want 1", n)
		}
	})
}

// An M1 database has a tenant, a channel and a key but no users.
func TestBootstrapUpgradesM1Database(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, TenantName, 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, DefaultChannel, 1000); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		log := slog.New(slog.NewTextHandler(&buf, nil))
		created, err := Bootstrap(ctx, db, failingKeys{}, log, time.Now(), "admin@localhost")
		if err != nil || !created {
			t.Fatalf("Bootstrap = %v, %v; want created", created, err)
		}
		if !adminLine.MatchString(buf.String()) || keyLine.MatchString(buf.String()) {
			t.Fatalf("log = %q; want the admin line and no key line", buf.String())
		}
		if n, _ := store.CountTenants(ctx, db.SQL, db.Dialect); n != 1 {
			t.Fatalf("tenants = %d, want 1", n)
		}
		buf.Reset()
		if created, err := Bootstrap(ctx, db, failingKeys{}, log, time.Now(), "admin@localhost"); err != nil || created || buf.Len() != 0 {
			t.Fatalf("second Bootstrap = %v, %v, log %q; want no-op", created, err, buf.String())
		}
	})
}

func TestResetAdmin(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		now := time.Now()
		tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, TenantName, 1, 1000)
		if err != nil {
			t.Fatal(err)
		}
		svc := &service.Service{DB: db}

		// No users at all: the admin is created.
		email, pw, err := ResetAdmin(ctx, db, tn.ID, "", now)
		if err != nil || email != "admin@localhost" || len(pw) != 20 {
			t.Fatalf("ResetAdmin on empty = %q, %q, %v", email, pw, err)
		}
		admin, err := svc.Login(ctx, tn.ID, email, pw)
		if err != nil || !admin.MustChangePassword {
			t.Fatalf("Login = %+v, %v", admin, err)
		}
		sess := store.Session{ID: "s1", TenantID: tn.ID, UserID: admin.ID, ExpiresAt: now.Add(time.Hour).UnixMilli(), CreatedAt: 1}
		if err := store.CreateSession(ctx, db.SQL, db.Dialect, sess); err != nil {
			t.Fatal(err)
		}
		rev, err := store.CreateUser(ctx, db.SQL, db.Dialect, store.User{
			TenantID: tn.ID, Email: "rev@x.test", PasswordHash: admin.PasswordHash, Role: "reviewer", CreatedAt: 2, UpdatedAt: 2})
		if err != nil {
			t.Fatal(err)
		}

		// The first admin gets a new password, a forced change, and no sessions.
		email2, pw2, err := ResetAdmin(ctx, db, tn.ID, "", now)
		if err != nil || email2 != email || pw2 == pw {
			t.Fatalf("ResetAdmin = %q, %q, %v", email2, pw2, err)
		}
		if _, err := svc.Login(ctx, tn.ID, email, pw); service.KindOf(err) != service.KindUnauthenticated {
			t.Fatalf("old password: err = %v", err)
		}
		if u, err := svc.Login(ctx, tn.ID, email, pw2); err != nil || !u.MustChangePassword {
			t.Fatalf("new password: %+v, %v", u, err)
		}
		if _, _, err := store.GetSessionUser(ctx, db.SQL, db.Dialect, "s1"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("session after reset: %v, want ErrNotFound", err)
		}

		// By email, case-insensitively; a reviewer is refused; an unknown email too.
		if got, _, err := ResetAdmin(ctx, db, tn.ID, " ADMIN@localhost", now); err != nil || got != email {
			t.Fatalf("ResetAdmin by email = %q, %v", got, err)
		}
		if _, _, err := ResetAdmin(ctx, db, tn.ID, rev.Email, now); err == nil || err.Error() != "user is not an admin" {
			t.Fatalf("ResetAdmin reviewer = %v", err)
		}
		if _, _, err := ResetAdmin(ctx, db, tn.ID, "ghost@x.test", now); err == nil {
			t.Fatal("ResetAdmin unknown email: want an error")
		}
	})
}
