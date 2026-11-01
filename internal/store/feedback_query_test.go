package store_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func TestListFeedbackFilters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, main := newTenant(t, db, "acme")
		web, _ := store.CreateChannel(ctx, db.SQL, d, tn.ID, "web", 1)
		other, och := newTenant(t, db, "other")

		seedFB(t, db, tn, main.ID, "fb_1", "Login is BROKEN", ptr("u1"), 100)
		seedFB(t, db, tn, main.ID, "fb_2", "love the app", ptr("u2"), 200)
		seedFB(t, db, tn, web.ID, "fb_3", "Broken search", ptr("u1"), 300)
		seedFB(t, db, tn, web.ID, "fb_4", "dark mode please", nil, 400)
		seedFB(t, db, other, och.ID, "fb_x", "broken elsewhere", ptr("u1"), 250)
		mustExec(t, db, "UPDATE feedback SET status = 'reviewed' WHERE id = ?", "fb_2")
		mustExec(t, db, "UPDATE feedback SET status = 'actioned' WHERE id = ?", "fb_3")

		tags, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Billing"}, 5)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_4"}, []string{tags[0].ID}); err != nil {
			t.Fatal(err)
		}

		cases := []struct {
			name string
			f    store.FeedbackFilter
			want []string
		}{
			{"none", store.FeedbackFilter{}, []string{"fb_4", "fb_3", "fb_2", "fb_1"}},
			{"one status", store.FeedbackFilter{Statuses: []string{"new"}}, []string{"fb_4", "fb_1"}},
			{"multi status", store.FeedbackFilter{Statuses: []string{"reviewed", "actioned"}}, []string{"fb_3", "fb_2"}},
			{"channel", store.FeedbackFilter{ChannelID: web.ID}, []string{"fb_4", "fb_3"}},
			{"tag other case", store.FeedbackFilter{Tag: "BILLING"}, []string{"fb_4", "fb_1"}},
			{"unknown tag", store.FeedbackFilter{Tag: "nope"}, nil},
			{"user", store.FeedbackFilter{UserID: "u1"}, []string{"fb_3", "fb_1"}},
			{"from inclusive", store.FeedbackFilter{From: i64(300)}, []string{"fb_4", "fb_3"}},
			{"to exclusive", store.FeedbackFilter{To: i64(300)}, []string{"fb_2", "fb_1"}},
			{"range", store.FeedbackFilter{From: i64(200), To: i64(400)}, []string{"fb_3", "fb_2"}},
			{"q case-insensitive", store.FeedbackFilter{Q: "bROKen"}, []string{"fb_3", "fb_1"}},
			{"combined", store.FeedbackFilter{Statuses: []string{"new"}, ChannelID: web.ID, Tag: "billing", UserID: "", Q: "dark"}, []string{"fb_4"}},
			{"combined empty", store.FeedbackFilter{Statuses: []string{"actioned"}, UserID: "u2"}, nil},
		}
		for _, c := range cases {
			got, err := store.ListFeedback(ctx, db.SQL, d, tn.ID, c.f, nil, 50)
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
				continue
			}
			wantIDs(t, c.name, got, c.want...)
		}
		// Tenant isolation: the other tenant sees only its own row.
		got, _ := store.ListFeedback(ctx, db.SQL, d, other.ID, store.FeedbackFilter{}, nil, 50)
		wantIDs(t, "other tenant", got, "fb_x")
		if _, err := store.GetFeedback(ctx, db.SQL, d, other.ID, "fb_1"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant GetFeedback err = %v", err)
		}
		// The other tenant's tag filter must not match this tenant's tag.
		got, _ = store.ListFeedback(ctx, db.SQL, d, other.ID, store.FeedbackFilter{Tag: "billing"}, nil, 50)
		wantIDs(t, "other tenant tag", got)
	})
}

func TestListFeedbackReadsFields(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		r := store.FeedbackRow{
			ID: "fb_1", TenantID: tn.ID, ChannelID: ch.ID, Message: "line1\nline2", UserID: ptr("u1"),
			AppVersion: ptr("1.0"), Platform: ptr("ios"), Metadata: []byte(`{"a":1}`), IdempotencyKey: "k", ReceivedAt: 42,
		}
		if _, err := store.InsertFeedbackBatch(ctx, db.SQL, d, []store.FeedbackRow{r}); err != nil {
			t.Fatal(err)
		}
		seedFB(t, db, tn, ch.ID, "fb_2", "no meta", nil, 41)
		got, err := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_1")
		if err != nil {
			t.Fatal(err)
		}
		if got.ChannelName == nil || *got.ChannelName != "main" || got.Message != "line1\nline2" || *got.AppVersion != "1.0" ||
			*got.Platform != "ios" || got.Status != "new" || got.ReceivedAt != 42 || got.UpdatedBy != nil || len(got.Tags) != 0 || got.Tags == nil {
			t.Errorf("got %+v", got)
		}
		if string(got.Metadata) != `{"a":1}` && string(got.Metadata) != `{"a": 1}` {
			t.Errorf("metadata = %q", got.Metadata)
		}
		g2, _ := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_2")
		if g2.Metadata != nil || g2.UserID != nil {
			t.Errorf("fb_2 = %+v", g2)
		}
	})
}

func TestListFeedbackLikeEscaping(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		seedFB(t, db, tn, ch.ID, "fb_1", "got 50% off", nil, 1)
		seedFB(t, db, tn, ch.ID, "fb_2", "got 500 off", nil, 2)
		seedFB(t, db, tn, ch.ID, "fb_3", "a_b here", nil, 3)
		seedFB(t, db, tn, ch.ID, "fb_4", "axb here", nil, 4)
		seedFB(t, db, tn, ch.ID, "fb_5", "wow! really", nil, 5)
		seedFB(t, db, tn, ch.ID, "fb_6", "wow really", nil, 6)
		for _, c := range []struct {
			q    string
			want []string
		}{
			{"50%", []string{"fb_1"}},
			{"a_b", []string{"fb_3"}},
			{"!", []string{"fb_5"}},
			{"w!", []string{"fb_5"}},
			{"%", []string{"fb_1"}},
			{"_", []string{"fb_3"}},
		} {
			got, err := store.ListFeedback(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{Q: c.q}, nil, 50)
			if err != nil {
				t.Fatalf("%q: %v", c.q, err)
			}
			wantIDs(t, c.q, got, c.want...)
		}
	})
}

func TestListFeedbackCursor(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		at := map[string]int64{"fb_a": 10, "fb_b": 20, "fb_c": 20, "fb_d": 30, "fb_e": 40, "fb_f": 40, "fb_g": 50}
		for id, ts := range at {
			seedFB(t, db, tn, ch.ID, id, "m", nil, ts)
		}
		want := []string{"fb_g", "fb_f", "fb_e", "fb_d", "fb_c", "fb_b", "fb_a"}
		var seen []string
		var cur *store.Cursor
		for pages := 0; pages < 10; pages++ {
			page, err := store.ListFeedback(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{}, cur, 3)
			if err != nil {
				t.Fatal(err)
			}
			if len(page) == 0 {
				break
			}
			seen = append(seen, idsOf(page)...)
			last := page[len(page)-1]
			cur = &store.Cursor{ReceivedAt: last.ReceivedAt, ID: last.ID}
		}
		if !reflect.DeepEqual(seen, want) {
			t.Errorf("seen %v, want %v", seen, want)
		}
	})
}

func TestListFeedbackTagsOneQuery(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		seedFB(t, db, tn, ch.ID, "fb_1", "m", nil, 1)
		seedFB(t, db, tn, ch.ID, "fb_2", "m", nil, 2)
		seedFB(t, db, tn, ch.ID, "fb_3", "m", nil, 3)
		tags, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"zeta", "Alpha", "mid"}, 5)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]string{}
		for _, tg := range tags {
			ids[tg.Name] = tg.ID
		}
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_3"}, []string{ids["zeta"], ids["Alpha"]})
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_3"}, []string{ids["mid"]})

		cq := &countingQ{Querier: db.SQL}
		got, err := store.ListFeedback(ctx, cq, d, tn.ID, store.FeedbackFilter{}, nil, 50)
		if err != nil {
			t.Fatal(err)
		}
		if cq.queries != 2 {
			t.Errorf("queries = %d, want 2 (rows + tags)", cq.queries)
		}
		wantTags := map[string][]string{"fb_1": {"Alpha", "zeta"}, "fb_2": {}, "fb_3": {"Alpha", "mid", "zeta"}}
		for _, f := range got {
			if f.Tags == nil || !reflect.DeepEqual(f.Tags, wantTags[f.ID]) {
				t.Errorf("%s tags = %#v, want %#v", f.ID, f.Tags, wantTags[f.ID])
			}
		}
		one, err := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_3")
		if err != nil || !reflect.DeepEqual(one.Tags, wantTags["fb_3"]) {
			t.Errorf("GetFeedback tags = %v, %v", one.Tags, err)
		}
	})
}

func TestSetFeedbackStatus(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		other, och := newTenant(t, db, "other")
		u := seedUser(t, db, tn.ID, "a@example.com", "admin", 1)
		seedFB(t, db, tn, ch.ID, "fb_1", "m", nil, 1)
		seedFB(t, db, tn, ch.ID, "fb_2", "m", nil, 2)
		seedFB(t, db, tn, ch.ID, "fb_3", "m", nil, 3)
		seedFB(t, db, other, och.ID, "fb_o", "m", nil, 4)
		mustExec(t, db, "UPDATE feedback SET status = 'reviewed' WHERE id = ?", "fb_2")

		changed, missing, err := store.SetFeedbackStatus(ctx, db.SQL, d, tn.ID,
			[]string{"fb_1", "fb_2", "fb_3", "fb_nope", "fb_o"}, "reviewed", u.ID, "mcp", 1000)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(changed, []string{"fb_1", "fb_3"}) || !reflect.DeepEqual(missing, []string{"fb_nope", "fb_o"}) {
			t.Errorf("changed=%v missing=%v", changed, missing)
		}
		for _, id := range []string{"fb_1", "fb_3"} {
			fb, _ := store.GetFeedback(ctx, db.SQL, d, tn.ID, id)
			if fb.Status != "reviewed" || fb.UpdatedAt != 1000 || fb.UpdatedBy == nil || *fb.UpdatedBy != u.ID {
				t.Errorf("%s = %+v", id, fb)
			}
		}
		fb2, _ := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_2")
		if fb2.UpdatedAt != 2 || fb2.UpdatedBy != nil {
			t.Errorf("unchanged row touched: %+v", fb2)
		}
		fbo, _ := store.GetFeedback(ctx, db.SQL, d, other.ID, "fb_o")
		if fbo.Status != "new" {
			t.Errorf("other tenant row changed: %+v", fbo)
		}
		if n := count(t, db, "status_changes"); n != 2 {
			t.Errorf("status_changes = %d, want 2", n)
		}
		hist, err := store.ListStatusChanges(ctx, db.SQL, d, tn.ID, "fb_1")
		if err != nil || len(hist) != 1 {
			t.Fatalf("hist = %+v, %v", hist, err)
		}
		h := hist[0]
		if h.ID[:3] != "sc_" || h.From != "new" || h.To != "reviewed" || h.Via != "mcp" || h.UserID != u.ID ||
			h.UserEmail == nil || *h.UserEmail != "a@example.com" || h.ChangedAt != 1000 || h.FeedbackID != "fb_1" {
			t.Errorf("hist[0] = %+v", h)
		}
		if h, _ := store.ListStatusChanges(ctx, db.SQL, d, other.ID, "fb_1"); len(h) != 0 {
			t.Errorf("cross-tenant history = %+v", h)
		}
		// Same status again: nothing changes.
		changed, _, err = store.SetFeedbackStatus(ctx, db.SQL, d, tn.ID, []string{"fb_1"}, "reviewed", u.ID, "ui", 2000)
		if err != nil || len(changed) != 0 || count(t, db, "status_changes") != 2 {
			t.Errorf("repeat: changed=%v err=%v", changed, err)
		}
	})
}

func TestEnsureTagsCaseInsensitive(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, _ := newTenant(t, db, "acme")
		other, _ := newTenant(t, db, "other")
		first, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Billing"}, 1)
		if err != nil || len(first) != 1 || first[0].Name != "Billing" || first[0].ID[:3] != "tg_" {
			t.Fatalf("first = %+v, %v", first, err)
		}
		again, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"billing", "BILLING", "Crash", "crash"}, 2)
		if err != nil || len(again) != 2 {
			t.Fatalf("again = %+v, %v", again, err)
		}
		if again[0].ID != first[0].ID || again[0].Name != "Billing" || again[1].Name != "Crash" {
			t.Errorf("again = %+v", again)
		}
		if n := count(t, db, "tags"); n != 2 {
			t.Errorf("tags = %d", n)
		}
		if e, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, nil, 3); err != nil || len(e) != 0 {
			t.Errorf("empty = %v, %v", e, err)
		}
		// Same name in another tenant is a different tag.
		o, err := store.EnsureTags(ctx, db.SQL, d, other.ID, []string{"billing"}, 3)
		if err != nil || len(o) != 1 || o[0].ID == first[0].ID || o[0].Name != "billing" {
			t.Errorf("other tenant = %+v, %v", o, err)
		}
		// The unique index rejects a raw case-variant duplicate.
		if err := exec(t, db, "INSERT INTO tags (id, tenant_id, name, created_at) VALUES (?, ?, ?, ?)", "tg_raw", tn.ID, "bILLING", 9); err == nil {
			t.Error("raw case-variant duplicate accepted")
		}
	})
}

func TestAddRemoveTags(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		other, och := newTenant(t, db, "other")
		seedFB(t, db, tn, ch.ID, "fb_1", "m", nil, 1)
		seedFB(t, db, tn, ch.ID, "fb_2", "m", nil, 2)
		seedFB(t, db, other, och.ID, "fb_o", "m", nil, 3)
		tags, _ := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Bug", "Idea", "Unused"}, 1)
		id := map[string]string{}
		for _, tg := range tags {
			id[tg.Name] = tg.ID
		}

		for i := 0; i < 2; i++ { // idempotent
			missing, err := store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_2", "fb_gone", "fb_o"}, []string{id["Bug"], id["Idea"]})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(missing, []string{"fb_gone", "fb_o"}) {
				t.Errorf("missing = %v", missing)
			}
		}
		if n := count(t, db, "feedback_tags"); n != 4 {
			t.Errorf("feedback_tags = %d, want 4", n)
		}
		ls, err := store.ListTags(ctx, db.SQL, d, tn.ID)
		if err != nil || len(ls) != 3 || ls[0].Name != "Bug" || ls[0].Count != 2 || ls[1].Name != "Idea" || ls[1].Count != 2 || ls[2].Name != "Unused" || ls[2].Count != 0 {
			t.Errorf("ListTags = %+v, %v", ls, err)
		}
		missing, err := store.RemoveFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_gone"}, []string{"bUG", "nonexistent"})
		if err != nil || !reflect.DeepEqual(missing, []string{"fb_gone"}) {
			t.Errorf("remove: missing=%v err=%v", missing, err)
		}
		fb1, _ := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_1")
		fb2, _ := store.GetFeedback(ctx, db.SQL, d, tn.ID, "fb_2")
		if !reflect.DeepEqual(fb1.Tags, []string{"Idea"}) || !reflect.DeepEqual(fb2.Tags, []string{"Bug", "Idea"}) {
			t.Errorf("tags fb1=%v fb2=%v", fb1.Tags, fb2.Tags)
		}
		ls, _ = store.ListTags(ctx, db.SQL, d, tn.ID)
		if ls[0].Name != "Bug" || ls[0].Count != 1 { // tag itself stays
			t.Errorf("after remove: %+v", ls)
		}
		// A tag ID from another tenant cannot be linked.
		ot, _ := store.EnsureTags(ctx, db.SQL, d, other.ID, []string{"foreign"}, 1)
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1"}, []string{ot[0].ID})
		if n := count(t, db, "feedback_tags"); n != 3 {
			t.Errorf("foreign tag linked: feedback_tags = %d", n)
		}
	})
}

func TestFeedbackStats(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, main := newTenant(t, db, "acme")
		web, _ := store.CreateChannel(ctx, db.SQL, d, tn.ID, "web", 1)
		other, och := newTenant(t, db, "other")
		seedFB(t, db, tn, main.ID, "fb_1", "m", nil, 100)
		seedFB(t, db, tn, main.ID, "fb_2", "m", nil, 200)
		seedFB(t, db, tn, web.ID, "fb_3", "m", nil, 300)
		seedFB(t, db, tn, web.ID, "fb_4", "m", nil, 400)
		seedFB(t, db, other, och.ID, "fb_o", "m", nil, 150)
		mustExec(t, db, "UPDATE feedback SET status = 'reviewed' WHERE id IN ('fb_2', 'fb_3')")
		mustExec(t, db, "UPDATE feedback SET status = 'ignored' WHERE id = 'fb_4'")
		tags, _ := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Bug", "Idea"}, 1)
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_2", "fb_3"}, []string{tags[0].ID})
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_4"}, []string{tags[1].ID})
		if err := store.DeleteChannel(ctx, db.SQL, d, tn.ID, web.ID); err != nil {
			t.Fatal(err)
		}

		s, err := store.GetFeedbackStats(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if s.Total != 4 || !reflect.DeepEqual(s.ByStatus, map[string]int64{"new": 1, "reviewed": 2, "actioned": 0, "ignored": 1}) {
			t.Errorf("total/status: %+v", s)
		}
		if len(s.ByChannel) != 2 || s.ByChannel[0].Count != 2 || s.ByChannel[1].Count != 2 {
			t.Fatalf("by channel: %+v", s.ByChannel)
		}
		for _, c := range s.ByChannel {
			switch c.ChannelID {
			case main.ID:
				if c.Name == nil || *c.Name != "main" {
					t.Errorf("main name = %v", c.Name)
				}
			case web.ID:
				if c.Name != nil {
					t.Errorf("deleted channel name = %v", *c.Name)
				}
			default:
				t.Errorf("unexpected channel %s", c.ChannelID)
			}
		}
		if len(s.ByTag) != 2 || s.ByTag[0].Name != "Bug" || s.ByTag[0].Count != 3 || s.ByTag[1].Name != "Idea" || s.ByTag[1].Count != 1 {
			t.Errorf("by tag: %+v", s.ByTag)
		}

		s, err = store.GetFeedbackStats(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{From: i64(200), To: i64(400)})
		if err != nil {
			t.Fatal(err)
		}
		if s.Total != 2 || s.ByStatus["reviewed"] != 2 || len(s.ByStatus) != 4 || len(s.ByTag) != 1 || s.ByTag[0].Count != 2 {
			t.Errorf("ranged: %+v", s)
		}
		s, _ = store.GetFeedbackStats(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{From: i64(1000)})
		if s.Total != 0 || len(s.ByStatus) != 4 || len(s.ByChannel) != 0 || len(s.ByTag) != 0 {
			t.Errorf("empty range: %+v", s)
		}
	})
}

func TestFeedbackStatsFilters(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, main := newTenant(t, db, "acme")
		web, _ := store.CreateChannel(ctx, db.SQL, d, tn.ID, "web", 1)
		other, och := newTenant(t, db, "other")
		seedFB(t, db, tn, main.ID, "fb_1", "Safari crash on login", ptr("u1"), 100)
		seedFB(t, db, tn, main.ID, "fb_2", "100% broken sync", ptr("u2"), 200)
		seedFB(t, db, tn, web.ID, "fb_3", "Dark mode idea", ptr("u1"), 300)
		seedFB(t, db, tn, web.ID, "fb_4", "Login slow on SAFARI", ptr("u1"), 400)
		seedFB(t, db, tn, web.ID, "fb_5", "plain note", nil, 500)
		seedFB(t, db, other, och.ID, "fb_o", "Safari crash", ptr("u1"), 150)
		mustExec(t, db, "UPDATE feedback SET status = 'reviewed' WHERE id = 'fb_2'")
		mustExec(t, db, "UPDATE feedback SET status = 'actioned' WHERE id = 'fb_3'")
		mustExec(t, db, "UPDATE feedback SET status = 'ignored' WHERE id = 'fb_4'")
		tags, err := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Bug", "Idea", "Safari"}, 1)
		if err != nil {
			t.Fatal(err)
		}
		bug, idea, saf := tags[0].ID, tags[1].ID, tags[2].ID
		for _, l := range []struct {
			ids []string
			tag string
		}{
			{[]string{"fb_1", "fb_2", "fb_4"}, bug},
			{[]string{"fb_3", "fb_4"}, idea},
			{[]string{"fb_1"}, saf},
		} {
			if _, err := store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, l.ids, []string{l.tag}); err != nil {
				t.Fatal(err)
			}
		}
		otags, _ := store.EnsureTags(ctx, db.SQL, d, other.ID, []string{"Bug"}, 1)
		store.AddFeedbackTags(ctx, db.SQL, d, other.ID, []string{"fb_o"}, []string{otags[0].ID})

		type tc struct {
			name     string
			f        store.FeedbackFilter
			total    int64
			status   [4]int64 // new, reviewed, actioned, ignored
			byTag    map[string]int64
			byChan   map[string]int64
			checkTag bool
			checkCh  bool
		}
		cases := []tc{
			{name: "tag", f: store.FeedbackFilter{Tag: "Bug"}, total: 3, status: [4]int64{1, 1, 0, 1},
				byTag: map[string]int64{"Bug": 3, "Idea": 1, "Safari": 1}, checkTag: true,
				byChan: map[string]int64{main.ID: 2, web.ID: 1}, checkCh: true},
			{name: "tag_case_insensitive", f: store.FeedbackFilter{Tag: "bUG"}, total: 3, status: [4]int64{1, 1, 0, 1}},
			{name: "channel", f: store.FeedbackFilter{ChannelID: web.ID}, total: 3, status: [4]int64{1, 0, 1, 1},
				byChan: map[string]int64{web.ID: 3}, checkCh: true,
				byTag: map[string]int64{"Idea": 2, "Bug": 1}, checkTag: true},
			{name: "userId", f: store.FeedbackFilter{UserID: "u1"}, total: 3, status: [4]int64{1, 0, 1, 1}},
			{name: "q", f: store.FeedbackFilter{Q: "safari"}, total: 2, status: [4]int64{1, 0, 0, 1}},
			{name: "q_escapes_like_wildcards", f: store.FeedbackFilter{Q: "%"}, total: 1, status: [4]int64{0, 1, 0, 0}},
			{name: "range", f: store.FeedbackFilter{From: i64(200), To: i64(400)}, total: 2, status: [4]int64{0, 1, 1, 0}},
			{name: "combined", f: store.FeedbackFilter{Tag: "bug", ChannelID: web.ID, Q: "safari", From: i64(300), To: i64(500)},
				total: 1, status: [4]int64{0, 0, 0, 1},
				byChan: map[string]int64{web.ID: 1}, checkCh: true,
				byTag: map[string]int64{"Bug": 1, "Idea": 1}, checkTag: true},
			{name: "status_ignored", f: store.FeedbackFilter{Statuses: []string{"new"}}, total: 5, status: [4]int64{2, 1, 1, 1}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				s, err := store.GetFeedbackStats(ctx, db.SQL, d, tn.ID, c.f)
				if err != nil {
					t.Fatal(err)
				}
				want := map[string]int64{"new": c.status[0], "reviewed": c.status[1], "actioned": c.status[2], "ignored": c.status[3]}
				if s.Total != c.total || !reflect.DeepEqual(s.ByStatus, want) {
					t.Errorf("total/status = %d %v, want %d %v", s.Total, s.ByStatus, c.total, want)
				}
				if c.checkTag {
					got := map[string]int64{}
					for _, tg := range s.ByTag {
						got[tg.Name] = tg.Count
					}
					if !reflect.DeepEqual(got, c.byTag) {
						t.Errorf("byTag = %v, want %v", got, c.byTag)
					}
				}
				if c.checkCh {
					got := map[string]int64{}
					for _, ch := range s.ByChannel {
						got[ch.ChannelID] = ch.Count
					}
					if !reflect.DeepEqual(got, c.byChan) {
						t.Errorf("byChannel = %v, want %v", got, c.byChan)
					}
				}
			})
		}
		t.Run("tenant_scoped", func(t *testing.T) {
			// fb_o matches u1, safari and tag Bug in the other tenant; none of
			// it may leak into acme's counts.
			for name, f := range map[string]store.FeedbackFilter{
				"userId": {UserID: "u1"}, "q": {Q: "safari"}, "tag": {Tag: "Bug"},
			} {
				s, err := store.GetFeedbackStats(ctx, db.SQL, d, tn.ID, f)
				if err != nil {
					t.Fatal(err)
				}
				want := map[string]int64{"userId": 3, "q": 2, "tag": 3}[name]
				if s.Total != want {
					t.Errorf("%s: acme total = %d, want %d", name, s.Total, want)
				}
			}
			s, err := store.GetFeedbackStats(ctx, db.SQL, d, other.ID, store.FeedbackFilter{Tag: "Bug"})
			if err != nil {
				t.Fatal(err)
			}
			if s.Total != 1 || len(s.ByTag) != 1 || s.ByTag[0].Count != 1 || len(s.ByChannel) != 1 || s.ByChannel[0].ChannelID != och.ID {
				t.Errorf("other tenant: %+v", s)
			}
		})
	})
}

func TestDeleteFeedback(t *testing.T) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) {
		ctx, d := context.Background(), db.Dialect
		tn, ch := newTenant(t, db, "acme")
		other, och := newTenant(t, db, "other")
		u := seedUser(t, db, tn.ID, "a@example.com", "admin", 1)
		seedFB(t, db, tn, ch.ID, "fb_1", "m", ptr("u1"), 1)
		seedFB(t, db, tn, ch.ID, "fb_2", "m", ptr("u1"), 2)
		seedFB(t, db, tn, ch.ID, "fb_3", "m", ptr("u2"), 3)
		seedFB(t, db, other, och.ID, "fb_o", "m", ptr("u1"), 4)
		tags, _ := store.EnsureTags(ctx, db.SQL, d, tn.ID, []string{"Bug"}, 1)
		store.AddFeedbackTags(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_2"}, []string{tags[0].ID})
		store.SetFeedbackStatus(ctx, db.SQL, d, tn.ID, []string{"fb_1", "fb_2"}, "actioned", u.ID, "ui", 10)

		if err := store.DeleteFeedback(ctx, db.SQL, d, other.ID, "fb_1"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("cross-tenant delete err = %v", err)
		}
		if err := store.DeleteFeedback(ctx, db.SQL, d, tn.ID, "fb_1"); err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteFeedback(ctx, db.SQL, d, tn.ID, "fb_1"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("second delete err = %v", err)
		}
		if n := count(t, db, "feedback_tags"); n != 1 {
			t.Errorf("feedback_tags = %d, want 1", n)
		}
		if n := count(t, db, "status_changes"); n != 1 {
			t.Errorf("status_changes = %d, want 1", n)
		}
		n, err := store.DeleteFeedbackByUser(ctx, db.SQL, d, tn.ID, "u1")
		if err != nil || n != 1 { // fb_2 only; fb_1 already gone
			t.Errorf("by user = %d, %v", n, err)
		}
		if n := count(t, db, "feedback_tags") + count(t, db, "status_changes"); n != 0 {
			t.Errorf("cascade left %d rows", n)
		}
		left, _ := store.ListFeedback(ctx, db.SQL, d, tn.ID, store.FeedbackFilter{}, nil, 10)
		wantIDs(t, "tenant rows", left, "fb_3")
		left, _ = store.ListFeedback(ctx, db.SQL, d, other.ID, store.FeedbackFilter{}, nil, 10)
		wantIDs(t, "other tenant rows", left, "fb_o")
	})
}
