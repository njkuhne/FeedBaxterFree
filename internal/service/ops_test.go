package service

import (
	"context"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxter/internal/ingest"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/store"
	"github.com/njkuhne/feedbaxter/internal/store/storetest"
)

func TestOps(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		s.Now = time.Now // stream timestamps are real time

		_, err := s.Ops(ctx, f.revP)
		wantKind(t, err, KindForbidden)

		for i := 0; i < 2; i++ {
			if _, err := f.conn.JS.Publish(ctx, queue.SubjectIngest, []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := f.conn.JS.Publish(ctx, queue.SubjectDLQ, []byte("dead")); err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		dk, mk := ingest.DayKey(f.tenant.ID, now), ingest.MonthKey(f.tenant.ID, 15, now)
		if _, err := f.obj.Counters.Put(ctx, dk, []byte("5")); err != nil {
			t.Fatal(err)
		}
		if _, err := f.obj.Counters.Put(ctx, mk, []byte("9")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)

		o, err := s.Ops(ctx, f.adminP)
		mustNil(t, err)
		if o.Queue.Depth != 2 || o.Queue.OldestAgeSeconds <= 0 || o.Queue.DLQ != 1 {
			t.Fatalf("queue %+v", o.Queue)
		}
		if o.Counters.Day != 5 || o.Counters.Month != 9 || o.Counters.DayKey != dk || o.Counters.MonthKey != mk {
			t.Fatalf("counters %+v", o.Counters)
		}
		if o.DBSizeBytes <= 0 || o.Version != "test" || o.Mode != "all" || o.Dialect != string(db.Dialect) {
			t.Fatalf("ops %+v", o)
		}

		// A missing counter key reads as 0.
		if err := f.obj.Counters.Delete(ctx, dk); err != nil {
			t.Fatal(err)
		}
		o, err = s.Ops(ctx, f.adminP)
		mustNil(t, err)
		if o.Counters.Day != 0 {
			t.Fatalf("day %d", o.Counters.Day)
		}

		s.Queue = nil
		if _, err := s.Ops(ctx, f.adminP); err == nil || KindOf(err) != 0 {
			t.Fatalf("nil queue: %v", err)
		}
	})
}
