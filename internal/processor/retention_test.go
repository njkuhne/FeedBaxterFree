package processor_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/internal/processor"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func seedRows(t *testing.T, db *store.DB, tn store.Tenant, ch store.Channel, prefix string, n int, at int64) {
	t.Helper()
	rows := make([]store.FeedbackRow, n)
	for i := range rows {
		rows[i] = store.FeedbackRow{
			ID: store.NewID(store.PrefixFeedback), TenantID: tn.ID, ChannelID: ch.ID, Message: "m",
			IdempotencyKey: fmt.Sprintf("%s-%d", prefix, i), ReceivedAt: at,
		}
	}
	tx, err := db.SQL.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertFeedbackBatch(context.Background(), tx, db.Dialect, rows); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestRunRetention(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		c, o := queuetest.Embedded(t, queue.Limits{})
		tn, ch := newTenant(t, db)
		cfg := testCfg()
		now := time.Now()
		old := now.Add(-cfg.RetentionWindow - time.Hour).UnixMilli()
		fresh := now.Add(-cfg.RetentionWindow + time.Hour).UnixMilli()
		seedRows(t, db, tn, ch, "old", 2500, old)
		seedRows(t, db, tn, ch, "new", 3, fresh)

		p := processor.New(db, c.JS, o.Consumer, cfg, nil)
		n, err := p.RunRetention(context.Background(), now)
		if err != nil || n != 2500 {
			t.Fatalf("deleted %d, err %v", n, err)
		}
		if got := countRows(t, db); got != 3 {
			t.Errorf("remaining = %d, want 3", got)
		}
		if n, err := p.RunRetention(context.Background(), now); err != nil || n != 0 {
			t.Errorf("second run deleted %d, err %v", n, err)
		}
	})
}

func TestRetentionLoopFires(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		c, o := queuetest.Embedded(t, queue.Limits{})
		tn, ch := newTenant(t, db)
		cfg := testCfg()
		cfg.RetentionCheckInterval = 50 * time.Millisecond
		seedRows(t, db, tn, ch, "old", 1, time.Now().Add(-cfg.RetentionWindow-time.Hour).UnixMilli())

		p := processor.New(db, c.JS, o.Consumer, cfg, nil)
		startProcessor(t, p)
		waitFor(t, "old row deleted", func() bool { return countRows(t, db) == 0 })
	})
}
