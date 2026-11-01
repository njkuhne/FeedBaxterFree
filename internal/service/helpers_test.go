package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/ingest/keysync"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxter/internal/store"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type noLimits struct{}

func (noLimits) Limits(context.Context, string) (ext.Limits, error) {
	return ext.Limits{Daily: 100, Monthly: 1000}, nil
}

type fixture struct {
	svc      *Service
	db       *store.DB
	obj      *queue.Objects
	conn     *queue.Conn
	clock    *fakeClock
	tenant   store.Tenant
	channel  store.Channel
	admin    store.User
	reviewer store.User
	adminP   ext.Principal
	revP     ext.Principal
	pwHash   string
	pw       string
}

const testPassword = "correct horse battery"

var (
	hashOnce sync.Once
	hashVal  string
)

func testHash(t testing.TB) string {
	hashOnce.Do(func() {
		var err error
		hashVal, err = HashPassword(testPassword)
		if err != nil {
			t.Fatal(err)
		}
	})
	return hashVal
}

func principal(u store.User, via string) ext.Principal {
	return ext.Principal{UserID: u.ID, TenantID: u.TenantID, Role: ext.Role(u.Role), Via: via}
}

func newFixture(t *testing.T, db *store.DB) *fixture {
	t.Helper()
	ctx := context.Background()
	conn, obj := queuetest.Embedded(t, queue.Limits{})
	clock := &fakeClock{t: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f := &fixture{db: db, obj: obj, conn: conn, clock: clock, pw: testPassword, pwHash: testHash(t)}
	f.tenant = f.newTenant(t, "Acme")
	f.channel = f.newChannel(t, f.tenant.ID, "web")
	f.admin = f.newUser(t, f.tenant.ID, "admin@acme.test", "admin")
	f.reviewer = f.newUser(t, f.tenant.ID, "rev@acme.test", "reviewer")
	f.adminP, f.revP = principal(f.admin, "ui"), principal(f.reviewer, "ui")
	f.svc = &Service{
		DB: db, Queue: obj, Now: clock.now, Log: log, Version: "test", Mode: "all",
		Keys: &keysync.Writer{DB: db, KV: obj.Keys, Limits: noLimits{}, Log: log},
	}
	_ = ctx
	return f
}

func (f *fixture) ms() int64 { return f.clock.now().UnixMilli() }

func (f *fixture) newTenant(t *testing.T, name string) store.Tenant {
	t.Helper()
	tn, err := store.CreateTenant(context.Background(), f.db.SQL, f.db.Dialect, name, 15, f.ms())
	if err != nil {
		t.Fatal(err)
	}
	return tn
}

func (f *fixture) newChannel(t *testing.T, tenantID, name string) store.Channel {
	t.Helper()
	c, err := store.CreateChannel(context.Background(), f.db.SQL, f.db.Dialect, tenantID, name, f.ms())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fixture) newUser(t *testing.T, tenantID, email, role string) store.User {
	t.Helper()
	u, err := store.CreateUser(context.Background(), f.db.SQL, f.db.Dialect, store.User{
		TenantID: tenantID, Email: email, PasswordHash: f.pwHash, Role: role, CreatedAt: f.ms(), UpdatedAt: f.ms(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// seed inserts one feedback row per message directly (the processor's job in production).
func (f *fixture) seed(t *testing.T, tenantID, channelID string, msgs ...string) []string {
	t.Helper()
	rows := make([]store.FeedbackRow, len(msgs))
	ids := make([]string, len(msgs))
	base := f.ms()
	for i, m := range msgs {
		ids[i] = fmt.Sprintf("fb_%s_%05d", tenantID[len(tenantID)-4:], i+1)
		rows[i] = store.FeedbackRow{
			ID: ids[i], TenantID: tenantID, ChannelID: channelID, Message: m,
			IdempotencyKey: ids[i], ReceivedAt: base + int64(i),
		}
	}
	if _, err := store.InsertFeedbackBatch(context.Background(), f.db.SQL, f.db.Dialect, rows); err != nil {
		t.Fatal(err)
	}
	return ids
}

func wantKind(t *testing.T, err error, k Kind) {
	t.Helper()
	if got := KindOf(err); got != k {
		t.Fatalf("kind = %d (err %v), want %d", got, err, k)
	}
}

func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
