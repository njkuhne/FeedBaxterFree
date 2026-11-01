package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func built() fstest.MapFS {
	return fstest.MapFS{
		".gitkeep":            {Data: nil},
		"index.html":          {Data: []byte("<!doctype html><title>x</title>")},
		"favicon.svg":         {Data: []byte("<svg/>")},
		"assets/app-1.js":     {Data: []byte("console.log(1)")},
		"assets/app-1.css":    {Data: []byte("a{}")},
		"assets/f-1.woff2":    {Data: []byte("wOF2")},
		"fonts/OFL-Inter.txt": {Data: []byte("licence")},
	}
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestAssetsImmutable(t *testing.T) {
	rec := do(Handler(built()), "GET", "/assets/app-1.js")
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if ct := do(Handler(built()), "GET", "/assets/app-1.css").Header().Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Fatalf("css type = %q", ct)
	}
}

func TestMissingAssetIs404(t *testing.T) {
	rec := do(Handler(built()), "GET", "/assets/nope.js")
	if rec.Code != 404 {
		t.Fatalf("code = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "<!doctype") {
		t.Fatal("missing asset served the SPA")
	}
}

func TestSPAFallbackNoStore(t *testing.T) {
	for _, p := range []string{"/", "/inbox/x", "/feedback/fb_1", "/index.html"} {
		rec := do(Handler(built()), "GET", p)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("%s: code %d body %q", p, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: Cache-Control = %q", p, got)
		}
		if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Fatalf("%s: Content-Type = %q", p, got)
		}
	}
}

func TestRootFilesNoCache(t *testing.T) {
	for _, p := range []string{"/favicon.svg", "/fonts/OFL-Inter.txt"} {
		rec := do(Handler(built()), "GET", p)
		if rec.Code != 200 {
			t.Fatalf("%s: code = %d", p, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
			t.Fatalf("%s: Cache-Control = %q", p, got)
		}
	}
	if ct := do(Handler(built()), "GET", "/favicon.svg").Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("svg type = %q", ct)
	}
}

func TestReservedPrefixesNotSPA(t *testing.T) {
	for _, p := range []string{"/v1/x", "/mcp", "/mcp/x", "/api", "/api/nope", "/v1"} {
		rec := do(Handler(built()), "GET", p)
		if rec.Code != 404 {
			t.Fatalf("%s: code = %d", p, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("%s: Content-Type = %q", p, ct)
		}
		if !strings.Contains(rec.Body.String(), `"error":"not_found"`) {
			t.Fatalf("%s: body = %q", p, rec.Body.String())
		}
	}
}

func TestDotfile404(t *testing.T) {
	for _, p := range []string{"/.gitkeep", "/assets/.hidden", "/.env"} {
		if rec := do(Handler(built()), "GET", p); rec.Code != 404 {
			t.Fatalf("%s: code = %d", p, rec.Code)
		}
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := do(Handler(built()), "POST", "/")
	if rec.Code != 405 {
		t.Fatalf("code = %d", rec.Code)
	}
	if rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("Allow = %q", rec.Header().Get("Allow"))
	}
	if rec := do(Handler(built()), "HEAD", "/"); rec.Code != 200 || rec.Body.Len() != 0 {
		t.Fatalf("HEAD = %d, %d bytes", rec.Code, rec.Body.Len())
	}
}

func TestWoff2ContentType(t *testing.T) {
	rec := do(Handler(built()), "GET", "/assets/f-1.woff2")
	if got := rec.Header().Get("Content-Type"); got != "font/woff2" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestSecurityHeaders(t *testing.T) {
	for _, p := range []string{"/", "/assets/app-1.js", "/assets/missing.js", "/v1/x"} {
		rec := do(Handler(built()), "GET", p)
		h := rec.Header()
		if h.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s: nosniff missing", p)
		}
		c := h.Get("Content-Security-Policy")
		if !strings.Contains(c, "default-src 'self'") || !strings.Contains(c, "script-src 'self'") ||
			!strings.Contains(c, "frame-ancestors 'none'") {
			t.Fatalf("%s: CSP = %q", p, c)
		}
		if h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") != "same-origin" {
			t.Fatalf("%s: framing/referrer headers missing", p)
		}
	}
}

func TestNotBuilt(t *testing.T) {
	h := Handler(fstest.MapFS{".gitkeep": {}})
	rec := do(h, "GET", "/")
	if rec.Code != 503 {
		t.Fatalf("code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Run make ui") {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if rec := do(h, "GET", "/inbox"); rec.Code != 503 {
		t.Fatalf("/inbox = %d", rec.Code)
	}
}
