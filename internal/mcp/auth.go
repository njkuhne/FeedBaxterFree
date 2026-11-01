package mcp

import (
	"net/http"
	"strings"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/service"
)

// bearerToken extracts the token from an "Authorization: Bearer <token>" value.
func bearerToken(header string) string {
	scheme, rest, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(rest)
}

// authenticate resolves the request's personal access token. On failure it
// has already written the response and returns false. Cookies are never read.
func (h *handler) authenticate(w http.ResponseWriter, r *http.Request) (ext.Principal, bool) {
	tok := bearerToken(r.Header.Get("Authorization"))
	if tok == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Send a personal access token in the Authorization header.")
		return ext.Principal{}, false
	}
	p, err := h.svc.PrincipalFromToken(r.Context(), tok)
	if err == nil {
		return p, true
	}
	if service.KindOf(err) == service.KindUnauthenticated {
		w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
		writeError(w, http.StatusUnauthorized, "unauthenticated", err.Error())
		return ext.Principal{}, false
	}
	h.log.Error("mcp: resolve token", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
	return ext.Principal{}, false
}
