package api_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/njkuhne/feedbaxterfree/internal/api"
)

func TestLoginFlowForcedChange(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		e.newUser("new@acme.test", "reviewer", true)
		login := func(email, pw string) reply {
			return e.req("POST", "/api/auth/login", map[string]string{"email": email, "password": pw})
		}

		e.wantErr(login("new@acme.test", "wrong password!"), 401, "unauthenticated")
		e.wantErr(login("nobody@acme.test", testPassword), 401, "unauthenticated")

		r := e.want(login("New@Acme.test", testPassword), 200)
		c := e.sessionCookie(r)
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Secure || c.Path != "/" || len(c.Value) < 40 {
			t.Fatalf("cookie = %+v", c)
		}
		if u := r.obj(t)["user"].(map[string]any); u["email"] != "new@acme.test" || u["role"] != "reviewer" || u["mustChangePassword"] != true {
			t.Fatalf("login body = %s", r.body)
		}
		if strings.Contains(string(r.body), "hash") || strings.Contains(string(r.body), "argon") {
			t.Fatalf("login body leaks a hash: %s", r.body)
		}
		tok := c.Value

		e.wantErr(e.req("GET", "/api/feedback", nil, cookie(tok)), 403, "password_change_required")
		e.wantErr(e.req("GET", "/api/nothing-here", nil, cookie(tok)), 403, "password_change_required")
		// Signing in again (e.g. as someone else) is not blocked by a must-change cookie.
		// The new login replaces the old session: the old token stops working.
		relog := e.want(e.req("POST", "/api/auth/login", map[string]string{"email": "new@acme.test", "password": testPassword}, cookie(tok)), 200)
		e.wantErr(e.req("GET", "/api/me", nil, cookie(tok)), 401, "unauthenticated")
		tok = e.sessionCookie(relog).Value
		meReply := e.want(e.req("GET", "/api/me", nil, cookie(tok)), 200)
		// /api/me renews the browser-side expiry of the same session cookie.
		if rc := e.sessionCookie(meReply); rc.Value != tok || rc.MaxAge != 30*24*3600 || !rc.HttpOnly {
			t.Fatalf("/api/me refreshed cookie = %+v", rc)
		}
		me := meReply.obj(t)
		if me["mustChangePassword"] != true || me["role"] != "reviewer" || len(me) != 4 {
			t.Fatalf("/api/me = %v", me)
		}

		change := func(cur, next string) reply {
			return e.req("POST", "/api/auth/password", map[string]string{"currentPassword": cur, "newPassword": next}, cookie(tok))
		}
		e.wantErr(change("not the password", "a brand new password"), 400, "invalid_request")
		e.wantErr(change(testPassword, "short"), 400, "invalid_request")
		e.want(change(testPassword, "a brand new password"), 204)

		// The session survives the change and the gate is gone.
		e.want(e.req("GET", "/api/feedback", nil, cookie(tok)), 200)
		if me := e.want(e.req("GET", "/api/me", nil, cookie(tok)), 200).obj(t); me["mustChangePassword"] != false {
			t.Fatalf("/api/me after change = %v", me)
		}
		e.wantErr(login("new@acme.test", testPassword), 401, "unauthenticated")
		e.want(login("new@acme.test", "a brand new password"), 200)
	})
}

func TestSecureCookieBehindTLSProxy(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		body := map[string]string{"email": e.admin.Email, "password": testPassword}
		for proto, secure := range map[string]bool{"https": true, "HTTPS, http": true, "http": false, "": false} {
			r := e.want(e.req("POST", "/api/auth/login", body, header("X-Forwarded-Proto", proto)), 200)
			if c := e.sessionCookie(r); c.Secure != secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
				t.Fatalf("X-Forwarded-Proto %q: cookie = %+v, want Secure=%v", proto, c, secure)
			}
		}
	})
}

func TestCSRF(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		bare := header("X-Requested-With", "")
		login := map[string]string{"email": e.admin.Email, "password": "wrong password!"}

		e.wantErr(e.req("POST", "/api/auth/login", login, bare), 403, "csrf")
		e.wantErr(e.req("POST", "/api/auth/logout", nil, bare), 403, "csrf")
		e.wantErr(e.asAdmin("POST", "/api/channels", map[string]string{"name": "x"}, bare), 403, "csrf")
		e.wantErr(e.asAdmin("DELETE", "/api/channels/"+e.channel.ID, nil, bare), 403, "csrf")
		e.wantErr(e.req("POST", "/api/nope", nil, bare), 403, "csrf")
		e.wantErr(e.req("POST", "/api/auth/logout", nil, bare, header("Sec-Fetch-Site", "cross-site")), 403, "csrf")
		e.wantErr(e.req("POST", "/api/auth/logout", nil, bare, header("Sec-Fetch-Site", "same-site")), 403, "csrf")

		e.want(e.req("POST", "/api/auth/logout", nil, bare, header("Sec-Fetch-Site", "same-origin")), 204)
		e.want(e.req("POST", "/api/auth/logout", nil, bare, header("Sec-Fetch-Site", "none")), 204)
		e.want(e.req("POST", "/api/auth/logout", nil), 204) // X-Requested-With
		e.wantErr(e.req("POST", "/api/auth/login", login), 401, "unauthenticated")

		// Safe methods need no proof.
		e.want(e.asAdmin("GET", "/api/me", nil, bare), 200)
		e.want(e.asAdmin("HEAD", "/api/me", nil, bare), 200)
	})
}

func TestLogoutEndsSession(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		e.want(e.asAdmin("GET", "/api/me", nil), 200)
		r := e.want(e.asAdmin("POST", "/api/auth/logout", nil), 204)
		if c := e.sessionCookie(r); c.MaxAge >= 0 || c.Value != "" || !c.HttpOnly {
			t.Fatalf("cleared cookie = %+v", c)
		}
		e.wantErr(e.asAdmin("GET", "/api/me", nil), 401, "unauthenticated")
		e.want(e.req("POST", "/api/auth/logout", nil), 204) // no session is fine
	})
}

func TestPATNotAcceptedOnAPI(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		_, secret, err := e.svc.CreateToken(context.Background(), e.principal(e.admin), "mcp")
		if err != nil {
			t.Fatal(err)
		}
		e.wantErr(e.req("GET", "/api/me", nil, header("Authorization", "Bearer "+secret)), 401, "unauthenticated")
		e.wantErr(e.req("GET", "/api/feedback", nil, header("Authorization", "Bearer "+secret)), 401, "unauthenticated")
		e.wantErr(e.req("GET", "/api/me", nil, header("Authorization", secret)), 401, "unauthenticated")
		e.wantErr(e.req("GET", "/api/me", nil, cookie(secret)), 401, "unauthenticated")
		if strings.Contains(e.log.String(), secret) {
			t.Fatal("the token appears in the log")
		}
	})
}

func TestReviewerForbidden(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		routes := []struct {
			method, path string
			body         any
		}{
			{"GET", "/api/users", nil},
			{"POST", "/api/users", map[string]string{"email": "x@acme.test", "role": "reviewer"}},
			{"PATCH", "/api/users/" + e.reviewer.ID, map[string]string{"role": "admin"}},
			{"DELETE", "/api/users/" + e.admin.ID, nil},
			{"POST", "/api/users/" + e.admin.ID + "/reset-password", nil},
			{"POST", "/api/channels", map[string]string{"name": "x"}},
			{"PATCH", "/api/channels/" + e.channel.ID, map[string]string{"name": "x"}},
			{"DELETE", "/api/channels/" + e.channel.ID, nil},
			{"GET", "/api/keys", nil},
			{"POST", "/api/channels/" + e.channel.ID + "/keys", nil},
			{"POST", "/api/keys/key_x/revoke", nil},
			{"DELETE", "/api/feedback/fb_0001", nil},
			{"POST", "/api/feedback/delete-by-user", map[string]string{"userId": "u1"}},
			{"GET", "/api/ops", nil},
		}
		for _, rt := range routes {
			t.Run(rt.method+" "+rt.path, func(t *testing.T) {
				e.wantErr(e.asReviewer(rt.method, rt.path, rt.body), 403, "forbidden")
			})
		}
		// What a reviewer may do.
		e.seed("one")
		e.want(e.asReviewer("GET", "/api/feedback", nil), 200)
		e.want(e.asReviewer("PATCH", "/api/feedback/fb_0001", map[string]string{"status": "reviewed"}), 200)
		e.want(e.asReviewer("GET", "/api/channels", nil), 200)
		e.want(e.asReviewer("GET", "/api/feedback/export?format=json", nil), 200)
		e.want(e.asReviewer("GET", "/api/tokens", nil), 200)
	})
}

func TestEndpointSmoke(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ids := e.seed("alpha", "beta", "gamma")
		var secrets []string

		me := e.want(e.asAdmin("GET", "/api/me", nil), 200).obj(t)
		if me["id"] != e.admin.ID || me["email"] != "admin@acme.test" || me["role"] != "admin" || me["mustChangePassword"] != false {
			t.Fatalf("me = %v", me)
		}

		// People.
		r := e.want(e.asAdmin("POST", "/api/users", map[string]string{"email": "Pat@Acme.test", "role": "reviewer"}), 201).obj(t)
		pat := r["user"].(map[string]any)
		temp := r["temporaryPassword"].(string)
		secrets = append(secrets, temp)
		if len(temp) != 20 || pat["email"] != "pat@acme.test" || pat["role"] != "reviewer" || pat["mustChangePassword"] != true || pat["createdAt"] == nil {
			t.Fatalf("create user = %v", r)
		}
		patID := pat["id"].(string)
		users := itemsOf(t, e.want(e.asAdmin("GET", "/api/users", nil), 200))
		if len(users) != 3 || users[0]["createdAt"] == nil {
			t.Fatalf("users = %v", users)
		}
		if u := e.want(e.asAdmin("PATCH", "/api/users/"+patID, map[string]string{"role": "admin"}), 200).obj(t); u["role"] != "admin" || u["id"] != patID {
			t.Fatalf("set role = %v", u)
		}
		r = e.want(e.asAdmin("POST", "/api/users/"+patID+"/reset-password", nil), 200).obj(t)
		secrets = append(secrets, r["temporaryPassword"].(string))
		if len(secrets[1]) != 20 || len(r) != 1 {
			t.Fatalf("reset password = %v", r)
		}
		e.want(e.asAdmin("DELETE", "/api/users/"+patID, nil), 204)
		e.wantErr(e.asAdmin("DELETE", "/api/users/"+patID, nil), 404, "not_found")

		// Tokens (own).
		r = e.want(e.asAdmin("POST", "/api/tokens", map[string]string{"name": "laptop"}), 201).obj(t)
		tk, secret := r["token"].(map[string]any), r["secret"].(string)
		secrets = append(secrets, secret)
		if !strings.HasPrefix(secret, "fbx_pat_") || tk["name"] != "laptop" || tk["prefix"] != secret[:12] || tk["lastUsedAt"] != nil || tk["createdAt"] == nil {
			t.Fatalf("create token = %v", r)
		}
		listed := e.want(e.asAdmin("GET", "/api/tokens", nil), 200)
		if strings.Contains(string(listed.body), secret) {
			t.Fatal("the token list shows the secret")
		}
		if got := itemsOf(t, listed); len(got) != 1 || got[0]["id"] != tk["id"] {
			t.Fatalf("tokens = %v", got)
		}
		e.want(e.asAdmin("DELETE", "/api/tokens/"+tk["id"].(string), nil), 204)

		// Channels and keys.
		ch := e.want(e.asAdmin("POST", "/api/channels", map[string]string{"name": "iOS"}), 201).obj(t)
		chID := ch["id"].(string)
		if ch["name"] != "iOS" || ch["createdAt"] == nil || len(ch) != 3 {
			t.Fatalf("channel = %v", ch)
		}
		e.wantErr(e.asAdmin("POST", "/api/channels", map[string]string{"name": "iOS"}), 409, "conflict")
		if c := e.want(e.asAdmin("PATCH", "/api/channels/"+chID, map[string]string{"name": "Apple"}), 200).obj(t); c["name"] != "Apple" || c["id"] != chID {
			t.Fatalf("rename = %v", c)
		}
		if got := itemsOf(t, e.want(e.asAdmin("GET", "/api/channels", nil), 200)); len(got) != 2 {
			t.Fatalf("channels = %v", got)
		}
		r = e.want(e.asAdmin("POST", "/api/channels/"+chID+"/keys", nil), 201).obj(t)
		key, ksecret := r["key"].(map[string]any), r["secret"].(string)
		secrets = append(secrets, ksecret)
		if !strings.HasPrefix(ksecret, "fbx_") || key["channelId"] != chID || key["revokedAt"] != nil || key["prefix"] != ksecret[:12] || key["createdAt"] == nil {
			t.Fatalf("create key = %v", r)
		}
		if got := itemsOf(t, e.want(e.asAdmin("GET", "/api/keys?channel="+chID, nil), 200)); len(got) != 1 || got[0]["id"] != key["id"] {
			t.Fatalf("keys = %v", got)
		}
		if got := itemsOf(t, e.want(e.asAdmin("GET", "/api/keys", nil), 200)); len(got) != 1 {
			t.Fatalf("all keys = %v", got)
		}
		if k := e.want(e.asAdmin("POST", "/api/keys/"+key["id"].(string)+"/revoke", nil), 200).obj(t); k["revokedAt"] == nil || k["id"] != key["id"] {
			t.Fatalf("revoke = %v", k)
		}
		e.wantErr(e.asAdmin("POST", "/api/keys/key_nope/revoke", nil), 404, "not_found")
		e.want(e.asAdmin("DELETE", "/api/channels/"+chID, nil), 204)

		// Feedback.
		list := e.want(e.asAdmin("GET", "/api/feedback", nil), 200).obj(t)
		first := list["items"].([]any)[0].(map[string]any)
		for _, k := range []string{"id", "channelId", "channelName", "message", "userId", "appVersion", "platform", "metadata", "status", "receivedAt", "updatedAt", "updatedBy", "tags"} {
			if _, ok := first[k]; !ok {
				t.Fatalf("feedback lacks %q: %v", k, first)
			}
		}
		if first["id"] != ids[2] || first["channelName"] != "web" || first["status"] != "new" || first["metadata"] != nil ||
			first["receivedAt"] != "2026-03-03T12:00:00.000Z" || len(first["tags"].([]any)) != 0 {
			t.Fatalf("first = %v", first)
		}
		if nc, ok := list["nextCursor"]; !ok || nc != nil {
			t.Fatalf("nextCursor = %v, want null", nc)
		}
		d := e.want(e.asAdmin("PATCH", "/api/feedback/"+ids[0], map[string]string{"status": "reviewed"}), 200).obj(t)
		if _, has := d["history"]; d["status"] != "reviewed" || d["updatedBy"] != e.admin.ID || has {
			t.Fatalf("patch = %v", d)
		}
		e.wantErr(e.asAdmin("PATCH", "/api/feedback/fb_missing", map[string]string{"status": "reviewed"}), 404, "not_found")
		e.wantErr(e.asAdmin("PATCH", "/api/feedback/"+ids[0], map[string]string{"status": "bogus"}), 400, "invalid_request")
		d = e.want(e.asAdmin("GET", "/api/feedback/"+ids[0], nil), 200).obj(t)
		hist := d["history"].([]any)
		if len(hist) != 1 || d["message"] != "alpha" {
			t.Fatalf("detail = %v", d)
		}
		if h := hist[0].(map[string]any); h["from"] != "new" || h["to"] != "reviewed" || h["userId"] != e.admin.ID || h["userEmail"] != e.admin.Email || h["via"] != "ui" || h["at"] == nil {
			t.Fatalf("history = %v", h)
		}
		e.wantErr(e.asAdmin("GET", "/api/feedback/fb_missing", nil), 404, "not_found")

		b := e.want(e.asAdmin("POST", "/api/feedback/status", map[string]any{"ids": []string{ids[1], ids[2], "fb_missing"}, "status": "actioned"}), 200).obj(t)
		if b["updated"] != float64(2) || fmt.Sprint(b["notFound"]) != "[fb_missing]" {
			t.Fatalf("bulk status = %v", b)
		}
		b = e.want(e.asAdmin("POST", "/api/feedback/add-tags", map[string]any{"ids": ids, "tags": []string{"bug", "ui"}}), 200).obj(t)
		if b["updated"] != float64(3) || len(b["notFound"].([]any)) != 0 {
			t.Fatalf("add tags = %v", b)
		}
		b = e.want(e.asAdmin("POST", "/api/feedback/remove-tags", map[string]any{"ids": ids[:1], "tags": []string{"ui"}}), 200).obj(t)
		if b["updated"] != float64(1) {
			t.Fatalf("remove tags = %v", b)
		}
		tags := itemsOf(t, e.want(e.asAdmin("GET", "/api/tags", nil), 200))
		if len(tags) != 2 || tags[0]["name"] != "bug" || tags[0]["count"] != float64(3) || tags[1]["count"] != float64(2) || tags[0]["id"] == "" {
			t.Fatalf("tags = %v", tags)
		}
		st := e.want(e.asAdmin("GET", "/api/stats", nil), 200).obj(t)
		bs := st["byStatus"].(map[string]any)
		if st["total"] != float64(3) || bs["reviewed"] != float64(1) || bs["actioned"] != float64(2) || bs["new"] != float64(0) {
			t.Fatalf("stats = %v", st)
		}
		if bc := st["byChannel"].([]any); len(bc) != 1 || bc[0].(map[string]any)["name"] != "web" || bc[0].(map[string]any)["count"] != float64(3) {
			t.Fatalf("byChannel = %v", bc)
		}
		if bt := st["byTag"].([]any); len(bt) != 2 || bt[0].(map[string]any)["name"] != "bug" {
			t.Fatalf("byTag = %v", bt)
		}
		st = e.want(e.asAdmin("GET", "/api/stats?from=2026-03-02T00:00:00Z&to=2026-03-03T00:00:00Z", nil), 200).obj(t)
		if st["total"] != float64(1) {
			t.Fatalf("ranged stats = %v", st)
		}
		e.wantErr(e.asAdmin("GET", "/api/stats?from=yesterday", nil), 400, "invalid_request")

		e.want(e.asAdmin("GET", "/api/feedback/export?format=csv", nil), 200)
		e.want(e.asAdmin("DELETE", "/api/feedback/"+ids[2], nil), 204)
		e.wantErr(e.asAdmin("DELETE", "/api/feedback/"+ids[2], nil), 404, "not_found")
		e.wantErr(e.asAdmin("POST", "/api/feedback/delete-by-user", map[string]string{"userId": ""}), 400, "invalid_request")
		if d := e.want(e.asAdmin("POST", "/api/feedback/delete-by-user", map[string]string{"userId": "nobody"}), 200).obj(t); d["deleted"] != float64(0) || len(d) != 1 {
			t.Fatalf("delete by user = %v", d)
		}

		e.want(e.asAdmin("GET", "/api/ops", nil), 200)

		// Secrets appear only in their creation responses, never in the log.
		for _, s := range secrets {
			if strings.Contains(e.log.String(), s) {
				t.Fatalf("a secret appears in the log:\n%s", e.log.String())
			}
		}
		if strings.Contains(e.log.String(), testPassword) {
			t.Fatal("a password appears in the log")
		}
	})
}

func TestFeedbackPagination(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ids := e.seed("1", "2", "3", "4", "5")
		var got []string
		cursor := ""
		for page := 1; page <= 3; page++ {
			path := "/api/feedback?limit=2"
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			r := e.want(e.asAdmin("GET", path, nil), 200)
			items := itemsOf(t, r)
			got = append(got, idsOf(items)...)
			next, _ := r.obj(t)["nextCursor"].(string)
			if page < 3 && (next == "" || len(items) != 2) {
				t.Fatalf("page %d: %d items, nextCursor %q", page, len(items), next)
			}
			if page == 3 && (next != "" || len(items) != 1) {
				t.Fatalf("last page: %d items, nextCursor %q", len(items), next)
			}
			cursor = next
		}
		if want := []string{ids[4], ids[3], ids[2], ids[1], ids[0]}; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
		e.wantErr(e.asAdmin("GET", "/api/feedback?limit=0", nil), 400, "invalid_request")
		e.wantErr(e.asAdmin("GET", "/api/feedback?limit=201", nil), 400, "invalid_request")
		e.wantErr(e.asAdmin("GET", "/api/feedback?limit=abc", nil), 400, "invalid_request")
		e.wantErr(e.asAdmin("GET", "/api/feedback?cursor=%21%21", nil), 400, "invalid_request")
	})
}

func TestListFilters(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		ids := e.seed("alpha crash", "beta slow", "gamma crash", "delta")
		p := e.principal(e.admin)
		for id, st := range map[string]string{ids[1]: "reviewed", ids[2]: "actioned", ids[3]: "ignored"} {
			if _, err := e.svc.SetStatus(ctx, p, []string{id}, st); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.svc.AddTags(ctx, p, []string{ids[0], ids[2]}, []string{"Bug"}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.SQL.ExecContext(ctx, e.db.Dialect.Rebind("UPDATE feedback SET user_id = ? WHERE id = ?"), "u-7", ids[1]); err != nil {
			t.Fatal(err)
		}

		list := func(query string) []string {
			return idsOf(itemsOf(t, e.want(e.asAdmin("GET", "/api/feedback?"+query, nil), 200)))
		}
		cases := []struct {
			query string
			want  []string
		}{
			{"status=new", []string{ids[0]}},
			{"status=new&status=reviewed", []string{ids[1], ids[0]}},
			{"status=new,reviewed", []string{ids[1], ids[0]}},
			{"status=new,reviewed&status=ignored", []string{ids[3], ids[1], ids[0]}},
			{"q=crash", []string{ids[2], ids[0]}},
			{"q=CRASH&status=actioned", []string{ids[2]}},
			{"tag=bug", []string{ids[2], ids[0]}},
			{"tag=nope", []string{}},
			{"userId=u-7", []string{ids[1]}},
			{"channel=" + e.channel.ID + "&status=ignored", []string{ids[3]}},
			{"from=2026-03-02T00:00:00Z", []string{ids[3], ids[2], ids[1]}},
			{"to=2026-03-03T00:00:00Z", []string{ids[1], ids[0]}},
			{"from=2026-03-02T00:00:00Z&to=2026-03-04T00:00:00Z", []string{ids[2], ids[1]}},
			{"from=2026-03-02T14:00:00%2B02:00&to=2026-03-03T14:00:00%2B02:00", []string{ids[1]}},
		}
		for _, c := range cases {
			if got := list(c.query); fmt.Sprint(got) != fmt.Sprint(c.want) {
				t.Errorf("?%s = %v, want %v", c.query, got, c.want)
			}
		}
		for _, bad := range []string{"status=bogus", "from=2026-03-02", "to=x", "from=2026-03-05T00:00:00Z&to=2026-03-01T00:00:00Z"} {
			e.wantErr(e.asAdmin("GET", "/api/feedback?"+bad, nil), 400, "invalid_request")
		}
		items := itemsOf(t, e.want(e.asAdmin("GET", "/api/feedback?tag=bug&limit=1", nil), 200))
		if fmt.Sprint(items[0]["tags"]) != "[Bug]" {
			t.Fatalf("tags = %v", items[0]["tags"])
		}
	})
}

func TestExportCSVQuoting(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ctx := context.Background()
		msgs := []string{"plain", "a,b", `say "hi"`, "line1\nline2", "=cmd|' /C calc'!A0", "+1", "-2", "@x", "\tTab", "ok - fine"}
		ids := e.seed(msgs...)
		p := e.principal(e.admin)
		if _, err := e.svc.SetStatus(ctx, p, ids[:2], "reviewed"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.svc.AddTags(ctx, p, ids[:1], []string{"bug", "ui"}); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.SQL.ExecContext(ctx, e.db.Dialect.Rebind(
			"UPDATE feedback SET user_id = ?, platform = ?, app_version = ?, metadata = ? WHERE id = ?"),
			"=u1", "ios", "1.2.3", `{"k":"v"}`, ids[0]); err != nil {
			t.Fatal(err)
		}

		r := e.want(e.asAdmin("GET", "/api/feedback/export?format=csv", nil), 200)
		if ct := r.header.Get("Content-Type"); ct != "text/csv; charset=utf-8" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if cd := r.header.Get("Content-Disposition"); !strings.HasPrefix(cd, `attachment; filename="feedback-`) || !strings.HasSuffix(cd, `.csv"`) || len(cd) != len(`attachment; filename="feedback-20060102.csv"`) {
			t.Fatalf("Content-Disposition = %q", cd)
		}
		if r.header.Get("Cache-Control") != "no-store" {
			t.Fatalf("Cache-Control = %q", r.header.Get("Cache-Control"))
		}
		raw := string(r.body)
		if strings.HasPrefix(raw, "\ufeff") || !strings.HasPrefix(raw, "id,received_at,status,channel,user_id,platform,app_version,tags,message,metadata\r\n") {
			t.Fatalf("header = %q", raw[:min(len(raw), 120)])
		}
		if strings.Contains(strings.ReplaceAll(raw, "\r\n", ""), "\n") {
			t.Fatalf("a bare LF outside CRLF: %q", raw)
		}
		recs, err := csv.NewReader(strings.NewReader(raw)).ReadAll()
		if err != nil {
			t.Fatalf("not valid CSV: %v\n%s", err, raw)
		}
		if len(recs) != len(msgs)+1 {
			t.Fatalf("%d records, want %d", len(recs), len(msgs)+1)
		}
		byID := map[string][]string{}
		for _, rec := range recs[1:] {
			byID[rec[0]] = rec
		}
		wantMsg := map[string]string{
			ids[0]: "plain", ids[1]: "a,b", ids[2]: `say "hi"`, ids[3]: "line1\nline2",
			ids[4]: "'=cmd|' /C calc'!A0", ids[5]: "'+1", ids[6]: "'-2", ids[7]: "'@x", ids[8]: "'\tTab", ids[9]: "ok - fine",
		}
		for id, m := range wantMsg {
			if byID[id][8] != m {
				t.Errorf("%s message = %q, want %q", id, byID[id][8], m)
			}
		}
		row := byID[ids[0]]
		if row[1] != "2026-03-01T12:00:00.000Z" || row[2] != "reviewed" || row[3] != "web" || row[4] != "'=u1" ||
			row[5] != "ios" || row[6] != "1.2.3" || row[7] != "bug; ui" || strings.ReplaceAll(row[9], " ", "") != `{"k":"v"}` { // jsonb respells it
			t.Errorf("row = %q", row)
		}
		if row := byID[ids[9]]; row[2] != "new" || row[4] != "" || row[9] != "" {
			t.Errorf("row = %q", row)
		}

		// The filter is honoured.
		r = e.want(e.asAdmin("GET", "/api/feedback/export?format=csv&status=reviewed", nil), 200)
		recs, err = csv.NewReader(bytes.NewReader(r.body)).ReadAll()
		if err != nil || len(recs) != 3 {
			t.Fatalf("filtered export: %d records, %v", len(recs), err)
		}
		// Nothing matches: header only.
		r = e.want(e.asAdmin("GET", "/api/feedback/export?format=csv&q=zzzz", nil), 200)
		if !strings.HasPrefix(string(r.body), "id,received_at") || strings.Count(string(r.body), "\r\n") != 1 {
			t.Fatalf("empty export = %q", r.body)
		}
		// Bad input never starts a download.
		for _, bad := range []string{"", "format=xml", "format=csv&status=bogus", "format=csv&from=x"} {
			r := e.asAdmin("GET", "/api/feedback/export?"+bad, nil)
			e.wantErr(r, 400, "invalid_request")
			if r.header.Get("Content-Disposition") != "" {
				t.Fatalf("?%s sent Content-Disposition", bad)
			}
		}
	})
}

func TestExportJSON(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		ids := e.seed("=not guarded", "two", "three")
		if _, err := e.svc.SetStatus(context.Background(), e.principal(e.admin), ids[:2], "ignored"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.SQL.ExecContext(context.Background(), e.db.Dialect.Rebind("UPDATE feedback SET metadata = ? WHERE id = ?"), `{"n":1}`, ids[0]); err != nil {
			t.Fatal(err)
		}
		r := e.want(e.asAdmin("GET", "/api/feedback/export?format=json", nil), 200)
		if r.header.Get("Content-Type") != "application/json" || !strings.Contains(r.header.Get("Content-Disposition"), `.json"`) {
			t.Fatalf("headers = %v", r.header)
		}
		var all []map[string]any
		if err := json.Unmarshal(r.body, &all); err != nil || len(all) != 3 {
			t.Fatalf("not a 3-element JSON array: %v\n%s", err, r.body)
		}
		last := all[2]
		if last["message"] != "=not guarded" || last["metadata"].(map[string]any)["n"] != float64(1) || last["status"] != "ignored" {
			t.Fatalf("item = %v", last)
		}

		r = e.want(e.asAdmin("GET", "/api/feedback/export?format=json&status=ignored", nil), 200)
		all = nil
		if err := json.Unmarshal(r.body, &all); err != nil || len(all) != 2 {
			t.Fatalf("filtered: %v\n%s", err, r.body)
		}
		r = e.want(e.asAdmin("GET", "/api/feedback/export?format=json&q=zzzz", nil), 200)
		if strings.TrimSpace(string(r.body)) != "[]" {
			t.Fatalf("empty export = %q", r.body)
		}
	})
}

// More rows than one page, so the flush and deadline path runs.
func TestExportManyRows(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		msgs := make([]string, 1203)
		for i := range msgs {
			msgs[i] = fmt.Sprintf("row %d", i)
		}
		e.seed(msgs...)
		r := e.want(e.asAdmin("GET", "/api/feedback/export?format=json", nil), 200)
		var all []map[string]any
		if err := json.Unmarshal(r.body, &all); err != nil || len(all) != len(msgs) {
			t.Fatalf("got %d items, %v", len(all), err)
		}
		r = e.want(e.asAdmin("GET", "/api/feedback/export?format=csv", nil), 200)
		if recs, err := csv.NewReader(bytes.NewReader(r.body)).ReadAll(); err != nil || len(recs) != len(msgs)+1 {
			t.Fatalf("got %d records, %v", len(recs), err)
		}
	})
}

func TestOpsShape(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		m := e.want(e.asAdmin("GET", "/api/ops", nil), 200).obj(t)
		q, c := m["queue"].(map[string]any), m["counters"].(map[string]any)
		for _, k := range []string{"depth", "oldestAgeSeconds", "dlq"} {
			if _, ok := q[k].(float64); !ok {
				t.Errorf("queue.%s = %v", k, q[k])
			}
		}
		for _, k := range []string{"day", "month"} {
			if _, ok := c[k].(float64); !ok {
				t.Errorf("counters.%s = %v", k, c[k])
			}
		}
		for _, k := range []string{"dayKey", "monthKey"} {
			if s, _ := c[k].(string); s == "" {
				t.Errorf("counters.%s = %v", k, c[k])
			}
		}
		if m["dbSizeBytes"].(float64) <= 0 || m["version"] != "test" || m["mode"] != "all" || m["dialect"] != string(e.db.Dialect) || len(m) != 6 {
			t.Fatalf("ops = %v", m)
		}
	})
}

func TestErrorShape(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		r := e.asAdmin("GET", "/api/nope", nil)
		e.wantErr(r, 404, "not_found")
		if ct := r.header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %q", ct)
		}
		if cc := r.header.Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("Cache-Control = %q", cc)
		}
		e.wantErr(e.asAdmin("GET", "/api/feedback/export/extra", nil), 404, "not_found")
		r = e.asAdmin("DELETE", "/api/me", nil)
		e.wantErr(r, 405, "method_not_allowed")
		if r.header.Get("Allow") == "" {
			t.Fatal("405 without Allow")
		}
		e.wantErr(e.req("GET", "/api/me", nil), 401, "unauthenticated")
		e.wantErr(e.asAdmin("POST", "/api/channels", `{"name":"x","bogus":1}`), 400, "invalid_request")
		e.wantErr(e.asAdmin("POST", "/api/channels", `{"name":`), 400, "invalid_request")
		e.wantErr(e.asAdmin("POST", "/api/channels", ``), 400, "invalid_request")
		e.wantErr(e.asAdmin("POST", "/api/channels", `{"name":"x"} {"name":"y"}`), 400, "invalid_request")
		e.wantErr(e.asAdmin("POST", "/api/channels", `{"name":7}`), 400, "invalid_request")
		e.wantErr(e.asAdmin("POST", "/api/channels", `{"name":""}`), 400, "invalid_request")
		big := `{"name":"` + strings.Repeat("x", 1<<20) + `"}`
		e.wantErr(e.asAdmin("POST", "/api/channels", big), 413, "payload_too_large")
		e.wantErr(e.asAdmin("POST", "/api/channels", map[string]string{"name": "web"}), 409, "conflict")
		e.wantErr(e.asAdmin("GET", "/api/feedback/fb_none", nil), 404, "not_found")
	})
}

func TestInternalErrorHidesDetail(t *testing.T) {
	eachDialect(t, func(t *testing.T, e *env) {
		// A stub authenticator keeps working after the database is gone.
		h := api.New(api.Config{Svc: e.svc, Auth: fixedAuth{p: e.principal(e.admin)}, Log: slog.New(slog.NewTextHandler(e.log, nil))})
		if err := e.db.SQL.Close(); err != nil {
			t.Fatal(err)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/channels", nil))
		if rec.Code != 500 || strings.TrimSpace(rec.Body.String()) != `{"error":"internal","message":"Something went wrong."}` {
			t.Fatalf("500 = %d %s", rec.Code, rec.Body)
		}
		if !strings.Contains(e.log.String(), "api: internal error") {
			t.Fatalf("the error was not logged:\n%s", e.log.String())
		}
	})
}
