package selfhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/ingest"
	"github.com/njkuhne/feedbaxter/internal/ingest/keysync"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
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

		created, err := Bootstrap(ctx, db, w, log, now)
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
		created, err = Bootstrap(ctx, db, w, log, now)
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
		created, err := Bootstrap(context.Background(), db, failingKeys{}, log, time.Now())
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
