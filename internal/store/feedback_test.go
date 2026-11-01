package store_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func ptr(s string) *string { return &s }

func newTenant(t *testing.T, db *store.DB, name string) (store.Tenant, store.Channel) {
	t.Helper()
	ctx := context.Background()
	tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, name, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, "main", 1000)
	if err != nil {
		t.Fatal(err)
	}
	return tn, ch
}

func fbRow(tn store.Tenant, ch store.Channel, key string, at int64) store.FeedbackRow {
	return store.FeedbackRow{
		ID: store.NewID(store.PrefixFeedback), TenantID: tn.ID, ChannelID: ch.ID,
		Message: "msg " + key, IdempotencyKey: key, ReceivedAt: at,
	}
}

func countFeedback(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.SQL.QueryRow("SELECT COUNT(*) FROM feedback").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInsertFeedbackBatch(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		rows := []store.FeedbackRow{fbRow(tn, ch, "a", 10), fbRow(tn, ch, "b", 20), fbRow(tn, ch, "c", 30)}
		rows[0].UserID, rows[0].AppVersion, rows[0].Platform = ptr("u1"), ptr("1.2.3"), ptr("ios")
		rows[0].Metadata = []byte(`{"k":"v","n":1}`)
		n, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, rows)
		if err != nil || n != 3 {
			t.Fatalf("n=%d err=%v", n, err)
		}
		for _, want := range rows {
			var (
				got                   store.FeedbackRow
				user, ver, plat, meta sql.NullString
				status                string
				received, updated     int64
			)
			err := db.SQL.QueryRow(db.Dialect.Rebind(
				"SELECT tenant_id, channel_id, message, user_id, metadata, app_version, platform, status, idempotency_key, received_at, updated_at FROM feedback WHERE id = ?"),
				want.ID).Scan(&got.TenantID, &got.ChannelID, &got.Message, &user, &meta, &ver, &plat, &status, &got.IdempotencyKey, &received, &updated)
			if err != nil {
				t.Fatal(err)
			}
			if got.TenantID != want.TenantID || got.ChannelID != want.ChannelID || got.Message != want.Message || got.IdempotencyKey != want.IdempotencyKey {
				t.Errorf("%s: got %+v", want.ID, got)
			}
			if status != "new" || received != want.ReceivedAt || updated != want.ReceivedAt {
				t.Errorf("%s: status=%q received=%d updated=%d", want.ID, status, received, updated)
			}
			for _, c := range []struct {
				name string
				got  sql.NullString
				want *string
			}{{"user_id", user, want.UserID}, {"app_version", ver, want.AppVersion}, {"platform", plat, want.Platform}} {
				if c.got.Valid != (c.want != nil) || (c.want != nil && c.got.String != *c.want) {
					t.Errorf("%s %s = %+v, want %v", want.ID, c.name, c.got, c.want)
				}
			}
			if want.Metadata == nil {
				if meta.Valid {
					t.Errorf("%s metadata = %q, want NULL", want.ID, meta.String)
				}
				continue
			}
			var a, b map[string]any
			if err := json.Unmarshal([]byte(meta.String), &a); err != nil {
				t.Fatal(err)
			}
			_ = json.Unmarshal(want.Metadata, &b)
			if fmt.Sprint(a) != fmt.Sprint(b) {
				t.Errorf("metadata = %v, want %v", a, b)
			}
		}
	})
}

func TestInsertFeedbackBatchDedupes(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		rows := []store.FeedbackRow{fbRow(tn, ch, "a", 1), fbRow(tn, ch, "b", 2), fbRow(tn, ch, "c", 3)}
		ins := func(rs []store.FeedbackRow) int {
			t.Helper()
			n, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, rs)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := ins(rows); n != 3 {
			t.Fatalf("first = %d", n)
		}
		// Same batch with fresh IDs: still duplicates by idempotency key.
		again := make([]store.FeedbackRow, len(rows))
		for i, r := range rows {
			r.ID = store.NewID(store.PrefixFeedback)
			again[i] = r
		}
		if n := ins(again); n != 0 {
			t.Errorf("repeat = %d", n)
		}
		if c := countFeedback(t, db); c != 3 {
			t.Errorf("rows = %d", c)
		}
		if n := ins([]store.FeedbackRow{fbRow(tn, ch, "d", 4), fbRow(tn, ch, "a", 5)}); n != 1 {
			t.Errorf("mixed = %d", n)
		}
		if n := ins([]store.FeedbackRow{fbRow(tn, ch, "e", 6), fbRow(tn, ch, "e", 7)}); n != 1 {
			t.Errorf("in-batch dup = %d", n)
		}
		tn2, ch2 := newTenant(t, db, "other")
		if n := ins([]store.FeedbackRow{fbRow(tn2, ch2, "a", 8)}); n != 1 {
			t.Errorf("other tenant = %d", n)
		}
	})
}

func TestInsertFeedbackBatchLarge(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		if n, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, nil); n != 0 || err != nil {
			t.Fatalf("empty: %d %v", n, err)
		}
		rows := make([]store.FeedbackRow, 2500)
		for i := range rows {
			rows[i] = fbRow(tn, ch, fmt.Sprintf("k%d", i), int64(i))
		}
		tx, err := db.SQL.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		n, err := store.InsertFeedbackBatch(ctx, tx, db.Dialect, rows)
		if err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if n != 2500 || countFeedback(t, db) != 2500 {
			t.Errorf("n=%d rows=%d", n, countFeedback(t, db))
		}
	})
}

func TestInsertFeedbackBatchInTxRollback(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		tx, err := db.SQL.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		n, err := store.InsertFeedbackBatch(ctx, tx, db.Dialect, []store.FeedbackRow{fbRow(tn, ch, "a", 1), fbRow(tn, ch, "b", 2)})
		if err != nil || n != 2 {
			t.Fatalf("n=%d err=%v", n, err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if c := countFeedback(t, db); c != 0 {
			t.Errorf("rows after rollback = %d", c)
		}
	})
}

func TestDeleteFeedbackOlderThan(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		tn, ch := newTenant(t, db, "acme")
		const base = int64(1_000_000)
		var rows []store.FeedbackRow
		for i, off := range []int64{-3, -2, -1, 0} {
			rows = append(rows, fbRow(tn, ch, fmt.Sprintf("k%d", i), base+off))
		}
		if _, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, rows); err != nil {
			t.Fatal(err)
		}
		n, err := store.DeleteFeedbackOlderThan(ctx, db.SQL, db.Dialect, base-1, 10)
		if err != nil || n != 2 {
			t.Fatalf("n=%d err=%v", n, err)
		}
		if c := countFeedback(t, db); c != 2 {
			t.Errorf("left = %d", c)
		}
		if n, err := store.DeleteFeedbackOlderThan(ctx, db.SQL, db.Dialect, base-1, 10); err != nil || n != 0 {
			t.Errorf("nothing old: n=%d err=%v", n, err)
		}

		var old []store.FeedbackRow
		for i := range 5 {
			old = append(old, fbRow(tn, ch, fmt.Sprintf("old%d", i), int64(i)))
		}
		if _, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, old); err != nil {
			t.Fatal(err)
		}
		if n, err := store.DeleteFeedbackOlderThan(ctx, db.SQL, db.Dialect, base-1, 1); err != nil || n != 1 {
			t.Errorf("limit 1: n=%d err=%v", n, err)
		}
		if c := countFeedback(t, db); c != 6 {
			t.Errorf("left = %d, want 6", c)
		}
	})
}
