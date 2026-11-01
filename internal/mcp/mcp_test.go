package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/mcp"
	"github.com/njkuhne/feedbaxterfree/internal/service"
	"github.com/njkuhne/feedbaxterfree/internal/store"
	"github.com/njkuhne/feedbaxterfree/internal/store/storetest"
)

func each(t *testing.T, fn func(t *testing.T, e *env)) {
	storetest.ForEachDialect(t, func(t *testing.T, db *store.DB) { fn(t, newEnv(t, db)) })
}

func jsonErr(t *testing.T, body string) (code, msg string) {
	t.Helper()
	var v struct{ Error, Message string }
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("body %q is not the error shape: %v", body, err)
	}
	return v.Error, v.Message
}

func TestUnauthenticated(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		unknown := "fbx_pat_" + strings.Repeat("a", 32)
		cases := []struct{ name, auth string }{
			{"no header", ""},
			{"basic", "Basic dXNlcjpwYXNz"},
			{"empty bearer", "Bearer "},
			{"malformed", "Bearer abc"},
			{"unknown", "Bearer " + unknown},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				hdr := map[string]string{"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
				if c.auth != "" {
					hdr["Authorization"] = c.auth
				}
				resp, body := raw(t, http.MethodPost, e.srv.URL+"/mcp", hdr, initBody)
				checkUnauth(t, resp, body)
			})
		}
		t.Run("get", func(t *testing.T) {
			resp, body := raw(t, http.MethodGet, e.srv.URL+"/mcp", nil, "")
			checkUnauth(t, resp, body)
		})
	})
}

func checkUnauth(t *testing.T, resp *http.Response, body string) {
	t.Helper()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", resp.StatusCode, body)
	}
	code, msg := jsonErr(t, body)
	if code != "unauthenticated" || msg == "" {
		t.Errorf("body = %s", body)
	}
	if !strings.HasPrefix(resp.Header.Get("WWW-Authenticate"), "Bearer") {
		t.Errorf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestSubPathNotFound(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		for _, p := range []string{"/mcp/x", "/mcp/"} {
			resp, body := raw(t, http.MethodPost, e.srv.URL+p,
				map[string]string{"Authorization": "Bearer " + e.adminTok, "Content-Type": "application/json"}, initBody)
			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("%s = %d, want 404", p, resp.StatusCode)
			}
			if code, _ := jsonErr(t, body); code != "not_found" {
				t.Errorf("%s code = %q", p, code)
			}
			if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Errorf("%s content type = %q", p, ct)
			}
		}
	})
}

func TestStatelessJSON(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		resp, body := e.postInit(t, e.adminTok)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, body)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Errorf("Content-Type = %q", ct)
		}
		if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
			t.Errorf("Mcp-Session-Id = %q, want none", id)
		}
		resp, _ = raw(t, http.MethodGet, e.srv.URL+"/mcp", map[string]string{"Authorization": "Bearer " + e.adminTok}, "")
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET = %d, want 405", resp.StatusCode)
		}
	})
}

func TestListTools(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		res, err := cs.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"add_tags", "get_feedback", "list_feedback", "list_tags", "remove_tags", "set_status", "stats"}
		var got []string
		for _, tl := range res.Tools {
			got = append(got, tl.Name)
			if tl.Description == "" {
				t.Errorf("%s: empty description", tl.Name)
			}
			in, _ := tl.InputSchema.(map[string]any)
			if in["type"] != "object" {
				t.Errorf("%s: input schema = %v", tl.Name, tl.InputSchema)
			}
			if tl.OutputSchema == nil {
				t.Errorf("%s: no output schema", tl.Name)
			}
			required := func() []any { r, _ := in["required"].([]any); return r }()
			switch tl.Name {
			case "set_status":
				if !slices.Contains(required, any("ids")) || !slices.Contains(required, any("status")) {
					t.Errorf("set_status required = %v", required)
				}
			case "list_feedback":
				if len(required) != 0 {
					t.Errorf("list_feedback required = %v", required)
				}
			}
		}
		slices.Sort(got)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("tools = %v, want %v", got, want)
		}
	})
}

func TestListFeedback(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		var all listOut
		ok(t, cs, "list_feedback", nil, &all)
		if want := []string{"fb_0005", "fb_0004", "fb_0003", "fb_0002", "fb_0001"}; !reflect.DeepEqual(ids(all.Items), want) {
			t.Fatalf("ids = %v, want %v", ids(all.Items), want)
		}
		if all.NextCursor != nil {
			t.Errorf("nextCursor = %v, want null", *all.NextCursor)
		}
		it := all.Items[2] // fb_0003
		if it.ReceivedAt != "2026-03-03T12:00:00.000Z" {
			t.Errorf("receivedAt = %q", it.ReceivedAt)
		}
		if m, _ := it.Metadata.(map[string]any); m["plan"] != "pro" {
			t.Errorf("metadata = %v", it.Metadata)
		}
		if all.Items[0].Metadata != nil {
			t.Errorf("metadata = %v, want null", all.Items[0].Metadata)
		}
		if it.Tags == nil || len(it.Tags) != 0 || it.Status != "new" || it.Message != "message 3\nsecond line" {
			t.Errorf("item = %+v", it)
		}
		if it.ChannelName == nil || *it.ChannelName != "web" {
			t.Errorf("channelName = %v", it.ChannelName)
		}

		// Paging.
		var seen []string
		cursor := ""
		for pages := 0; ; pages++ {
			if pages > 5 {
				t.Fatal("too many pages")
			}
			args := map[string]any{"limit": 2}
			if cursor != "" {
				args["cursor"] = cursor
			}
			var pg listOut
			ok(t, cs, "list_feedback", args, &pg)
			seen = append(seen, ids(pg.Items)...)
			if pg.NextCursor == nil {
				break
			}
			cursor = *pg.NextCursor
		}
		if !reflect.DeepEqual(seen, ids(all.Items)) {
			t.Errorf("paged ids = %v, want %v", seen, ids(all.Items))
		}

		// Filters.
		var b bulkRes
		ok(t, cs, "set_status", map[string]any{"ids": []string{"fb_0001"}, "status": "reviewed"}, &b)
		ok(t, cs, "add_tags", map[string]any{"ids": []string{"fb_0002"}, "tags": []string{"bug"}}, &b)
		check := func(name string, args map[string]any, want ...string) {
			t.Helper()
			var out listOut
			ok(t, cs, "list_feedback", args, &out)
			if !reflect.DeepEqual(ids(out.Items), want) {
				t.Errorf("%s: ids = %v, want %v", name, ids(out.Items), want)
			}
		}
		check("status", map[string]any{"status": []string{"reviewed"}}, "fb_0001")
		check("tag", map[string]any{"tag": "bug"}, "fb_0002")
		check("q", map[string]any{"q": "message 4"}, "fb_0004")
		check("channel", map[string]any{"channel": e.channel.ID, "limit": 1}, "fb_0005")
		check("range", map[string]any{"from": "2026-03-02T12:00:00Z", "to": "2026-03-05T12:00:00Z"}, "fb_0004", "fb_0003", "fb_0002")

		// Errors.
		fails(t, cs, "list_feedback", map[string]any{"limit": 201})
		if msg := fails(t, cs, "list_feedback", map[string]any{"from": "yesterday"}); msg != "Use an RFC 3339 date for from, for example 2026-03-01T00:00:00Z." {
			t.Errorf("from error = %q", msg)
		}
		if msg := fails(t, cs, "list_feedback", map[string]any{"to": "soon"}); !strings.Contains(msg, "for to,") {
			t.Errorf("to error = %q", msg)
		}
	})
}

func TestBadCursor(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		if msg := fails(t, cs, "list_feedback", map[string]any{"cursor": "!!!"}); msg != "Invalid cursor." {
			t.Errorf("text = %q", msg)
		}
	})
}

type detail struct {
	feedbackItem
	History []struct {
		From, To, UserID, Via, At string
		UserEmail                 *string
	} `json:"history"`
}

func TestGetFeedback(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		var b bulkRes
		ok(t, cs, "set_status", map[string]any{"ids": []string{"fb_0003"}, "status": "actioned"}, &b)
		ok(t, cs, "add_tags", map[string]any{"ids": []string{"fb_0003"}, "tags": []string{"ux"}}, &b)
		var d detail
		ok(t, cs, "get_feedback", map[string]any{"id": "fb_0003"}, &d)
		if d.ID != "fb_0003" || d.Status != "actioned" || !reflect.DeepEqual(d.Tags, []string{"ux"}) {
			t.Errorf("detail = %+v", d)
		}
		if m, _ := d.Metadata.(map[string]any); m["plan"] != "pro" {
			t.Errorf("metadata = %v", d.Metadata)
		}
		if len(d.History) != 1 || d.History[0].From != "new" || d.History[0].To != "actioned" {
			t.Errorf("history = %+v", d.History)
		}
		if msg := fails(t, cs, "get_feedback", map[string]any{"id": "fb_9999"}); msg != "Feedback not found." {
			t.Errorf("text = %q", msg)
		}
	})
}

func TestSetStatusViaMCP(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.revTok)
		var b bulkRes
		ok(t, cs, "set_status", map[string]any{"ids": []string{"fb_0001", "fb_0002", "fb_0099"}, "status": "ignored"}, &b)
		if b.Updated != 2 || !reflect.DeepEqual(b.NotFound, []string{"fb_0099"}) {
			t.Fatalf("result = %+v", b)
		}
		var d detail
		ok(t, cs, "get_feedback", map[string]any{"id": "fb_0001"}, &d)
		if len(d.History) != 1 {
			t.Fatalf("history = %+v", d.History)
		}
		h := d.History[0]
		if h.Via != "mcp" || h.UserID != e.reviewer.ID || h.UserEmail == nil || *h.UserEmail != e.reviewer.Email {
			t.Errorf("history entry = %+v", h)
		}
		if d.UpdatedBy == nil || *d.UpdatedBy != e.reviewer.ID {
			t.Errorf("updatedBy = %v", d.UpdatedBy)
		}
	})
}

type tagList struct {
	Items []struct {
		ID, Name string
		Count    int64
	} `json:"items"`
}

func TestTags(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		var b bulkRes
		ok(t, cs, "add_tags", map[string]any{"ids": []string{"fb_0001", "fb_0002"}, "tags": []string{"Billing"}}, &b)
		if b.Updated != 2 {
			t.Fatalf("add = %+v", b)
		}
		count := func() int64 {
			var tl tagList
			ok(t, cs, "list_tags", nil, &tl)
			for _, it := range tl.Items {
				if it.Name == "Billing" {
					return it.Count
				}
			}
			return 0
		}
		if n := count(); n != 2 {
			t.Fatalf("count = %d, want 2", n)
		}
		ok(t, cs, "remove_tags", map[string]any{"ids": []string{"fb_0001", "fb_0002"}, "tags": []string{"Billing"}}, &b)
		if b.Updated != 2 {
			t.Errorf("remove = %+v", b)
		}
		if n := count(); n != 0 {
			t.Errorf("count after remove = %d, want 0", n)
		}
		var out listOut
		ok(t, cs, "list_feedback", map[string]any{"tag": "Billing"}, &out)
		if len(out.Items) != 0 {
			t.Errorf("items = %v", ids(out.Items))
		}
		fails(t, cs, "add_tags", map[string]any{"ids": []string{"fb_0001"}, "tags": []string{strings.Repeat("x", 65)}})
	})
}

func TestStats(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		var b bulkRes
		ok(t, cs, "set_status", map[string]any{"ids": []string{"fb_0001"}, "status": "reviewed"}, &b)
		ok(t, cs, "add_tags", map[string]any{"ids": []string{"fb_0001", "fb_0002"}, "tags": []string{"bug"}}, &b)
		var st struct {
			Total     int64
			ByStatus  map[string]int64
			ByChannel []struct {
				ChannelID string
				Name      *string
				Count     int64
			}
			ByTag []struct {
				Name  string
				Count int64
			}
		}
		ok(t, cs, "stats", nil, &st)
		if st.Total != 5 || st.ByStatus["new"] != 4 || st.ByStatus["reviewed"] != 1 {
			t.Errorf("stats = %+v", st)
		}
		if len(st.ByChannel) != 1 || st.ByChannel[0].ChannelID != e.channel.ID || st.ByChannel[0].Name == nil ||
			*st.ByChannel[0].Name != "web" || st.ByChannel[0].Count != 5 {
			t.Errorf("byChannel = %+v", st.ByChannel)
		}
		if len(st.ByTag) != 1 || st.ByTag[0].Name != "bug" || st.ByTag[0].Count != 2 {
			t.Errorf("byTag = %+v", st.ByTag)
		}
		ok(t, cs, "stats", map[string]any{"from": "2026-03-02T00:00:00Z", "to": "2026-03-04T00:00:00Z"}, &st)
		if st.Total != 2 {
			t.Errorf("ranged total = %d, want 2", st.Total)
		}
		fails(t, cs, "stats", map[string]any{"from": "2026-03-05T00:00:00Z", "to": "2026-03-01T00:00:00Z"})
	})
}

func TestBulkLimit(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		gen := func(n int) []string {
			out := []string{"fb_0001"}
			for i := 1; len(out) < n; i++ {
				out = append(out, fmt.Sprintf("nope_%04d", i))
			}
			return out
		}
		calls := map[string]map[string]any{
			"set_status":  {"status": "reviewed"},
			"add_tags":    {"tags": []string{"x"}},
			"remove_tags": {"tags": []string{"x"}},
		}
		for name, extra := range calls {
			args := func(n int) map[string]any {
				m := map[string]any{"ids": gen(n)}
				for k, v := range extra {
					m[k] = v
				}
				return m
			}
			if msg := fails(t, cs, name, args(501)); !strings.Contains(msg, "500") {
				t.Errorf("%s 501: %q", name, msg)
			}
			var b bulkRes
			ok(t, cs, name, args(500), &b)
			if len(b.NotFound) != 499 {
				t.Errorf("%s 500: notFound = %d, want 499", name, len(b.NotFound))
			}
		}
	})
}

func TestReviewerCanUseEveryTool(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.revTok)
		ids := []string{"fb_0001"}
		calls := []struct {
			name string
			args map[string]any
		}{
			{"list_feedback", nil},
			{"get_feedback", map[string]any{"id": "fb_0001"}},
			{"list_tags", nil},
			{"set_status", map[string]any{"ids": ids, "status": "reviewed"}},
			{"add_tags", map[string]any{"ids": ids, "tags": []string{"t"}}},
			{"remove_tags", map[string]any{"ids": ids, "tags": []string{"t"}}},
			{"stats", nil},
		}
		for _, c := range calls {
			if res := call(t, cs, c.name, c.args); res.IsError {
				t.Errorf("%s: %s", c.name, text(res))
			}
		}
	})
}

func TestRevokedToken(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		cs := e.connect(e.adminTok)
		var tl tagList
		ok(t, cs, "list_tags", nil, &tl)
		if err := e.svc.RevokeToken(context.Background(), principalOf(e.admin), e.adminTID); err != nil {
			t.Fatal(err)
		}
		if _, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "list_tags"}); err == nil {
			t.Error("call after revoke succeeded")
		}
		resp, body := e.postInit(t, e.adminTok)
		checkUnauth(t, resp, body)
	})
}

func TestInternalErrorNotLeaked(t *testing.T) {
	// SQLite only: the shared test DB must stay open for the harness cleanup.
	db, err := store.Open(context.Background(), config.Database{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	buf := &syncBuf{}
	svc := &service.Service{DB: db, Log: slog.New(slog.NewTextHandler(buf, nil)), Version: "test"}
	srv := httptest.NewServer(mcp.New(svc, slog.New(slog.NewTextHandler(buf, nil))))
	defer srv.Close()
	tok := "fbx_pat_" + strings.Repeat("a", 32)
	resp, body := raw(t, http.MethodPost, srv.URL+"/mcp", map[string]string{
		"Authorization": "Bearer " + tok, "Content-Type": "application/json", "Accept": "application/json, text/event-stream",
	}, initBody)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if code, msg := jsonErr(t, body); code != "internal" || msg != "Something went wrong." {
		t.Errorf("body = %s", body)
	}
	if strings.Contains(strings.ToLower(body), "sql") || strings.Contains(body, "closed") {
		t.Errorf("body leaks driver text: %s", body)
	}
	logs := buf.String()
	if !strings.Contains(logs, "mcp: resolve token") || strings.Contains(logs, tok) {
		t.Errorf("log = %s", logs)
	}
}

func TestNoGoroutineGrowth(t *testing.T) {
	each(t, func(t *testing.T, e *env) {
		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		cs := e.connectWith(e.adminTok, tr)
		var tl tagList
		for range 5 {
			ok(t, cs, "list_tags", nil, &tl)
		}
		tr.CloseIdleConnections()
		time.Sleep(100 * time.Millisecond)
		base := runtime.NumGoroutine()
		for range 50 {
			ok(t, cs, "list_tags", nil, &tl)
		}
		tr.CloseIdleConnections()
		deadline := time.Now().Add(2 * time.Second)
		for {
			n := runtime.NumGoroutine()
			if n <= base+5 {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("goroutines = %d, baseline %d", n, base)
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
}
