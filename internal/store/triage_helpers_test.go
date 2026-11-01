package store_test

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/store"
)

func seedUser(t *testing.T, db *store.DB, tenantID, email, role string, at int64) store.User {
	t.Helper()
	u, err := store.CreateUser(context.Background(), db.SQL, db.Dialect, store.User{
		TenantID: tenantID, Email: email, PasswordHash: "h", Role: role, CreatedAt: at, UpdatedAt: at,
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

// seedFB inserts one feedback row and returns its ID.
func seedFB(t *testing.T, db *store.DB, tn store.Tenant, channelID, id, msg string, userID *string, at int64) string {
	t.Helper()
	r := store.FeedbackRow{
		ID: id, TenantID: tn.ID, ChannelID: channelID, Message: msg, UserID: userID,
		IdempotencyKey: "k-" + id, ReceivedAt: at,
	}
	if n, err := store.InsertFeedbackBatch(context.Background(), db.SQL, db.Dialect, []store.FeedbackRow{r}); err != nil || n != 1 {
		t.Fatalf("seed feedback %s: n=%d err=%v", id, n, err)
	}
	return id
}

func idsOf(fbs []store.Feedback) []string {
	out := []string{}
	for _, f := range fbs {
		out = append(out, f.ID)
	}
	return out
}

func wantIDs(t *testing.T, label string, got []store.Feedback, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if g := idsOf(got); !reflect.DeepEqual(g, want) {
		t.Errorf("%s: got %v, want %v", label, g, want)
	}
}

// countingQ counts QueryContext calls.
type countingQ struct {
	store.Querier
	queries int
}

func (c *countingQ) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	c.queries++
	return c.Querier.QueryContext(ctx, q, args...)
}

func i64(n int64) *int64 { return &n }
