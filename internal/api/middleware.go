package api

import (
	"net/http"

	"github.com/njkuhne/feedbaxterfree/ext"
)

// ServeHTTP applies, in order: Cache-Control, the CSRF check, principal
// resolution (cookie only; personal access tokens are for MCP and never work
// here), the must-change-password gate, then routing.
func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	if !safeMethod(r.Method) && !sameOrigin(r) {
		writeCode(w, http.StatusForbidden, codeCSRF, "This request was refused. Reload the page and try again.")
		return
	}
	if a.auth != nil {
		if p, ok := a.auth.Principal(r); ok {
			r = r.WithContext(ext.WithPrincipal(r.Context(), p))
			if p.MustChangePassword && !allowedWhileMustChange(r) {
				writeCode(w, http.StatusForbidden, codePasswordChange, "Change your password to continue.")
				return
			}
		}
	}
	if _, pattern := a.mux.Handler(r); pattern == "" {
		a.unrouted(w, r)
		return
	}
	a.mux.ServeHTTP(w, r)
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// sameOrigin is the CSRF rule: the browser says the request is same-origin (or
// user-initiated), or the caller set a custom header, which a cross-site page
// cannot do without a CORS preflight that we never grant.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	}
	return r.Header.Get("X-Requested-With") != ""
}

func allowedWhileMustChange(r *http.Request) bool {
	switch r.URL.Path {
	case "/api/auth/password", "/api/auth/logout", "/api/auth/login":
		return r.Method == http.MethodPost
	case "/api/me":
		return r.Method == http.MethodGet || r.Method == http.MethodHead
	}
	return false
}

// probe records what the mux's own 404 or 405 handler would send.
type probe struct {
	header http.Header
	status int
}

func (p *probe) Header() http.Header         { return p.header }
func (p *probe) Write(b []byte) (int, error) { return len(b), nil }
func (p *probe) WriteHeader(code int)        { p.status = code }

// unrouted answers a request no route matches with the JSON error shape.
func (a *api) unrouted(w http.ResponseWriter, r *http.Request) {
	h, _ := a.mux.Handler(r)
	pr := &probe{header: http.Header{}}
	h.ServeHTTP(pr, r)
	if pr.status == http.StatusMethodNotAllowed {
		if allow := pr.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		writeCode(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "That method is not allowed here.")
		return
	}
	writeCode(w, http.StatusNotFound, codeNotFound, "Not found.")
}
