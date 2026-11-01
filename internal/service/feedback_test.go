package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func TestCursorPagination(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		ids := f.seed(t, f.tenant.ID, f.channel.ID, "a", "b", "c", "d", "e", "f", "g")

		var got []string
		cursor := ""
		pages := 0
		for {
			pg, err := s.ListFeedback(ctx, f.revP, Filter{}, cursor, 3)
			mustNil(t, err)
			pages++
			for _, it := range pg.Items {
				got = append(got, it.ID)
			}
			if pg.NextCursor == "" {
				break
			}
			cursor = pg.NextCursor
		}
		if pages != 3 || len(got) != 7 {
			t.Fatalf("pages %d items %d", pages, len(got))
		}
		for i, id := range got { // newest first
			if id != ids[6-i] {
				t.Fatalf("order: got %v", got)
			}
		}
		// Exactly one full page has no next cursor.
		pg, err := s.ListFeedback(ctx, f.revP, Filter{}, "", 7)
		mustNil(t, err)
		if len(pg.Items) != 7 || pg.NextCursor != "" {
			t.Fatalf("full page: %d %q", len(pg.Items), pg.NextCursor)
		}
		pg, _ = s.ListFeedback(ctx, f.revP, Filter{}, "", 0)
		if len(pg.Items) != 7 {
			t.Fatal("default limit")
		}
	})
}

func TestListValidation(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		now := time.Now()
		earlier := now.Add(-time.Hour)
		cases := []struct {
			name   string
			f      Filter
			cursor string
			limit  int
		}{
			{"bad status", Filter{Statuses: []string{"done"}}, "", 10},
			{"limit 201", Filter{}, "", 201},
			{"negative limit", Filter{}, "", -1},
			{"bad cursor", Filter{}, "!!!", 10},
			{"cursor not json", Filter{}, "aGVsbG8", 10},
			{"from after to", Filter{From: &now, To: &earlier}, "", 10},
			{"q too long", Filter{Q: strings.Repeat("x", 201)}, "", 10},
		}
		for _, c := range cases {
			_, err := s.ListFeedback(ctx, f.revP, c.f, c.cursor, c.limit)
			if KindOf(err) != KindInvalid {
				t.Errorf("%s: err %v", c.name, err)
			}
		}
		_, err := s.Stats(ctx, f.revP, Filter{From: &now, To: &earlier})
		wantKind(t, err, KindInvalid)
		_, err = s.GetFeedback(ctx, f.revP, "fb_missing")
		wantKind(t, err, KindNotFound)
	})
}

func TestStatsFilters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		app := f.newChannel(t, f.tenant.ID, "app")
		base := f.ms()
		u1, u2 := "u1", "u2"
		rows := []store.FeedbackRow{
			{ID: "fb_s1", TenantID: f.tenant.ID, ChannelID: f.channel.ID, Message: "Safari crash", UserID: &u1, ReceivedAt: base},
			{ID: "fb_s2", TenantID: f.tenant.ID, ChannelID: f.channel.ID, Message: "love it", UserID: &u2, ReceivedAt: base + 1},
			{ID: "fb_s3", TenantID: f.tenant.ID, ChannelID: app.ID, Message: "safari slow", UserID: &u1, ReceivedAt: base + 2},
			{ID: "fb_s4", TenantID: f.tenant.ID, ChannelID: app.ID, Message: "dark mode", ReceivedAt: base + 3},
		}
		for i := range rows {
			rows[i].IdempotencyKey = rows[i].ID
		}
		if _, err := store.InsertFeedbackBatch(ctx, db.SQL, db.Dialect, rows); err != nil {
			t.Fatal(err)
		}
		_, err := s.SetStatus(ctx, f.revP, []string{"fb_s2"}, "reviewed")
		mustNil(t, err)
		_, err = s.AddTags(ctx, f.revP, []string{"fb_s1", "fb_s3"}, []string{"Bug"})
		mustNil(t, err)

		total := func(name string, fl Filter, want int64) store.FeedbackStats {
			t.Helper()
			st, err := s.Stats(ctx, f.revP, fl)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if st.Total != want {
				t.Errorf("%s: total = %d, want %d (%+v)", name, st.Total, want, st)
			}
			return st
		}
		total("none", Filter{}, 4)
		st := total("tag", Filter{Tag: "bug"}, 2)
		if st.ByStatus["new"] != 2 || len(st.ByTag) != 1 || st.ByTag[0].Count != 2 {
			t.Errorf("tag stats %+v", st)
		}
		st = total("channel", Filter{ChannelID: app.ID}, 2)
		if len(st.ByChannel) != 1 || st.ByChannel[0].ChannelID != app.ID {
			t.Errorf("channel stats %+v", st.ByChannel)
		}
		total("userId", Filter{UserID: "u1"}, 2)
		total("q", Filter{Q: "SAFARI"}, 2)
		total("combined", Filter{Tag: "bug", ChannelID: app.ID, Q: "safari", UserID: "u1"}, 1)
		st = total("status ignored", Filter{Statuses: []string{"reviewed"}}, 4)
		if st.ByStatus["new"] != 3 || st.ByStatus["reviewed"] != 1 {
			t.Errorf("status was applied: %+v", st.ByStatus)
		}

		_, err = s.Stats(ctx, f.revP, Filter{Q: strings.Repeat("x", 201)})
		wantKind(t, err, KindInvalid)
		now := f.clock.now()
		earlier := now.Add(-time.Hour)
		_, err = s.Stats(ctx, f.revP, Filter{From: &now, To: &earlier})
		wantKind(t, err, KindInvalid)
	})
}

func TestBulkLimits(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		ids := f.seed(t, f.tenant.ID, f.channel.ID, "one", "two")

		many := make([]string, 501)
		for i := range many {
			many[i] = fmt.Sprintf("fb_x%d", i)
		}
		_, err := s.SetStatus(ctx, f.revP, many, "reviewed")
		wantKind(t, err, KindInvalid)
		_, err = s.SetStatus(ctx, f.revP, nil, "reviewed")
		wantKind(t, err, KindInvalid)
		_, err = s.SetStatus(ctx, f.revP, ids, "done")
		wantKind(t, err, KindInvalid)
		bad := f.revP
		bad.Via = "cli"
		_, err = s.SetStatus(ctx, bad, ids, "reviewed")
		wantKind(t, err, KindInvalid)

		// Duplicates collapse; unknown ids are reported.
		res, err := s.SetStatus(ctx, f.revP, []string{ids[0], ids[0], "fb_nope", ids[1]}, "actioned")
		mustNil(t, err)
		if res.Updated != 2 || len(res.NotFound) != 1 || res.NotFound[0] != "fb_nope" {
			t.Fatalf("result %+v", res)
		}
		res, err = s.SetStatus(ctx, f.revP, ids, "new") // undo
		mustNil(t, err)
		if res.Updated != 2 || res.NotFound == nil || len(res.NotFound) != 0 {
			t.Fatalf("result %+v", res)
		}

		// Tags.
		_, err = s.AddTags(ctx, f.revP, ids, nil)
		wantKind(t, err, KindInvalid)
		_, err = s.AddTags(ctx, f.revP, ids, []string{" "})
		wantKind(t, err, KindInvalid)
		tooMany := make([]string, 21)
		for i := range tooMany {
			tooMany[i] = fmt.Sprintf("t%d", i)
		}
		_, err = s.AddTags(ctx, f.revP, ids, tooMany)
		wantKind(t, err, KindInvalid)
		res, err = s.AddTags(ctx, f.revP, append(ids, "fb_nope"), []string{" Billing ", "billing", "bug"})
		mustNil(t, err)
		if res.Updated != 2 || len(res.NotFound) != 1 {
			t.Fatalf("add %+v", res)
		}
		d, _ := s.GetFeedback(ctx, f.revP, ids[0])
		if len(d.Tags) != 2 {
			t.Fatalf("tags %v", d.Tags)
		}
		tags, _ := s.ListTags(ctx, f.revP)
		if len(tags) != 2 || tags[0].Count != 2 {
			t.Fatalf("list tags %+v", tags)
		}
		res, err = s.RemoveTags(ctx, f.revP, ids, []string{"BUG"})
		mustNil(t, err)
		if res.Updated != 2 {
			t.Fatalf("remove %+v", res)
		}
		d, _ = s.GetFeedback(ctx, f.revP, ids[0])
		if len(d.Tags) != 1 || d.Tags[0] != "Billing" {
			t.Fatalf("tags after remove %v", d.Tags)
		}
		st, err := s.Stats(ctx, f.revP, Filter{})
		mustNil(t, err)
		if st.Total != 2 || st.ByStatus["new"] != 2 {
			t.Fatalf("stats %+v", st)
		}
	})
}

func TestStatusHistoryVia(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		ids := f.seed(t, f.tenant.ID, f.channel.ID, "hello")
		mcp := f.revP
		mcp.Via = "mcp"
		_, err := s.SetStatus(ctx, f.revP, ids, "reviewed")
		mustNil(t, err)
		f.clock.advance(time.Second)
		_, err = s.SetStatus(ctx, mcp, ids, "actioned")
		mustNil(t, err)
		d, err := s.GetFeedback(ctx, f.adminP, ids[0])
		mustNil(t, err)
		if len(d.History) != 2 || d.History[0].Via != "ui" || d.History[1].Via != "mcp" ||
			d.History[1].From != "reviewed" || d.History[1].To != "actioned" || d.History[0].UserID != f.reviewer.ID {
			t.Fatalf("history %+v", d.History)
		}
		if d.Status != "actioned" {
			t.Fatalf("status %s", d.Status)
		}
	})
}

func TestExportHonoursFilters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		const n = 1200
		msgs := make([]string, n)
		for i := range msgs {
			if i%4 != 3 {
				msgs[i] = fmt.Sprintf("Dark mode request %d", i)
			} else {
				msgs[i] = fmt.Sprintf("something else %d", i)
			}
		}
		ids := f.seed(t, f.tenant.ID, f.channel.ID, msgs...)
		var reviewed []string
		for i, id := range ids {
			if i%5 == 0 {
				reviewed = append(reviewed, id)
			}
		}
		for start := 0; start < len(reviewed); start += MaxBulkIDs {
			_, err := s.SetStatus(ctx, f.revP, reviewed[start:min(start+MaxBulkIDs, len(reviewed))], "reviewed")
			mustNil(t, err)
		}
		var want []string
		for i := n - 1; i >= 0; i-- { // newest first
			if i%4 != 3 && i%5 != 0 {
				want = append(want, ids[i])
			}
		}
		if len(want) <= exportPageSize {
			t.Fatalf("test data too small to cross pages: %d", len(want))
		}
		var got []string
		err := s.Export(ctx, f.revP, Filter{Statuses: []string{"new"}, Q: "DARK MODE"}, func(fb store.Feedback) error {
			got = append(got, fb.ID)
			return nil
		})
		mustNil(t, err)
		if len(got) != len(want) {
			t.Fatalf("got %d rows, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("row %d: %s != %s", i, got[i], want[i])
			}
		}
		// fn error stops the export.
		stop := fmt.Errorf("stop")
		calls := 0
		err = s.Export(ctx, f.revP, Filter{}, func(store.Feedback) error { calls++; return stop })
		if err != stop || calls != 1 {
			t.Fatalf("err %v calls %d", err, calls)
		}
		// A done context stops it too.
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if err := s.Export(cctx, f.revP, Filter{}, func(store.Feedback) error { return nil }); err == nil {
			t.Fatal("cancelled context not honoured")
		}
		wantKind(t, s.Export(ctx, f.revP, Filter{Statuses: []string{"x"}}, nil), KindInvalid)
	})
}

func TestDeleteFeedback(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		ids := f.seed(t, f.tenant.ID, f.channel.ID, "a", "b", "c")
		mustNil(t, s.DeleteFeedback(ctx, f.adminP, ids[0]))
		wantKind(t, s.DeleteFeedback(ctx, f.adminP, ids[0]), KindNotFound)
		_, err := s.DeleteUserFeedback(ctx, f.adminP, "")
		wantKind(t, err, KindInvalid)
		n, err := s.DeleteUserFeedback(ctx, f.adminP, "nobody")
		mustNil(t, err)
		if n != 0 {
			t.Fatal("deleted rows for unknown user")
		}
	})
}

func TestDeleteDoesNotTouchCounters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx := context.Background()
		f := newFixture(t, db)
		s := f.svc
		ids := f.seed(t, f.tenant.ID, f.channel.ID, "a", "b")
		uid := "end-user-1"
		_, err := db.SQL.ExecContext(ctx, db.Dialect.Rebind("UPDATE feedback SET user_id = ? WHERE id = ?"), uid, ids[1])
		mustNil(t, err)

		key := "tenant.day.test"
		rev, err := f.obj.Counters.Put(ctx, key, []byte("42"))
		mustNil(t, err)

		mustNil(t, s.DeleteFeedback(ctx, f.adminP, ids[0]))
		n, err := s.DeleteUserFeedback(ctx, f.adminP, uid)
		mustNil(t, err)
		if n != 1 {
			t.Fatalf("deleted %d", n)
		}
		e, err := f.obj.Counters.Get(ctx, key)
		mustNil(t, err)
		if string(e.Value()) != "42" || e.Revision() != rev {
			t.Fatalf("counter changed: %s rev %d (was %d)", e.Value(), e.Revision(), rev)
		}
		keys, _ := f.obj.Counters.Keys(ctx)
		if len(keys) != 1 {
			t.Fatalf("counter keys %v", keys)
		}
	})
}
