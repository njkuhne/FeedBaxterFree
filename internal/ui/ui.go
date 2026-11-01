// Package ui serves the built admin UI: hashed assets with long caching, a
// single-page-app fallback to index.html, and strict security headers.
package ui

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

const csp = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; " +
	"frame-ancestors 'none'; form-action 'self'"

const notBuilt = "The admin UI is not built into this binary. Run make ui, then rebuild.\n"

// contentTypes covers what the UI build emits. Go's built-in table lacks woff2
// and the scratch image has no /etc/mime.types, so the types are set here.
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".svg":   "image/svg+xml",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
	".json":  "application/json",
	".png":   "image/png",
	".ico":   "image/x-icon",
	".map":   "application/json",
}

// Handler serves fsys, which is the already-sub'd dist directory.
func Handler(fsys fs.FS) http.Handler {
	return handler{fsys: fsys}
}

type handler struct{ fsys fs.FS }

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		jsonError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}
	p := path.Clean("/" + r.URL.Path)
	if reserved(p) {
		jsonError(w, r, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	for _, seg := range strings.Split(p, "/") {
		if strings.HasPrefix(seg, ".") {
			plainNotFound(w, r)
			return
		}
	}
	name := strings.TrimPrefix(p, "/")
	if strings.HasPrefix(p, "/assets/") {
		if !isFile(h.fsys, name) {
			plainNotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		serveFile(w, r, h.fsys, name)
		return
	}
	if name != "" && name != "index.html" && isFile(h.fsys, name) {
		w.Header().Set("Cache-Control", "no-cache")
		serveFile(w, r, h.fsys, name)
		return
	}
	if !isFile(h.fsys, "index.html") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusServiceUnavailable)
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, notBuilt)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	serveFile(w, r, h.fsys, "index.html")
}

func reserved(p string) bool {
	return p == "/api" || p == "/mcp" || strings.HasPrefix(p, "/api/") ||
		strings.HasPrefix(p, "/mcp/") || p == "/v1" || strings.HasPrefix(p, "/v1/")
}

func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", csp)
}

func isFile(fsys fs.FS, name string) bool {
	if !fs.ValidPath(name) {
		return false
	}
	st, err := fs.Stat(fsys, name)
	return err == nil && st.Mode().IsRegular()
}

func serveFile(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	f, err := fsys.Open(name)
	if err != nil {
		plainNotFound(w, r)
		return
	}
	defer f.Close()
	ct, ok := contentTypes[strings.ToLower(path.Ext(name))]
	if !ok {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if st, err := f.Stat(); err == nil {
		w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	}
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, f)
	}
}

func plainNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, "Not found.\n")
	}
}

func jsonError(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, `{"error":"`+code+`","message":"`+msg+`"}`+"\n")
	}
}
