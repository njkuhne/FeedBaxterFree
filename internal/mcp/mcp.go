// Package mcp serves the Model Context Protocol endpoint at /mcp: seven
// feedback tools over Streamable HTTP, authenticated with personal access
// tokens. The tools are thin wrappers over internal/service, which owns every
// role check, limit and the "via" attribution.
package mcp

import (
	"encoding/json"
	"log/slog"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/service"
)

const (
	path         = "/mcp"
	maxBodyBytes = 1 << 20
)

type handler struct {
	svc *service.Service
	log *slog.Logger
	sdk http.Handler
}

// New returns the handler for exactly the path /mcp. It builds one sdk.Server
// with the seven tools and one sdk.StreamableHTTPHandler, both shared by every
// request. log nil = slog.Default().
func New(svc *service.Service, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	h := &handler{svc: svc, log: log}
	srv := sdk.NewServer(
		&sdk.Implementation{Name: "feedbaxter", Title: "FeedBaxter", Version: svc.Version},
		&sdk.ServerOptions{Instructions: "Read and triage the app feedback in this FeedBaxter workspace."},
	)
	h.addTools(srv)
	h.sdk = sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return srv }, &sdk.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		Logger:                       log,
		MaxRequestBodyBytes:          maxBodyBytes,
		PropagateRequestCancellation: true,
		// Every request carries a bearer token, which a DNS-rebinding page cannot
		// have. The guard would only break reverse proxies on the same machine,
		// which connect to 127.0.0.1 with the public Host header.
		DisableLocalhostProtection: true,
	})
	return h
}

// ServeHTTP answers /mcp: no-store, exact path, bearer token, then the SDK.
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path != path {
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	p, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	h.sdk.ServeHTTP(w, r.WithContext(ext.WithPrincipal(r.Context(), p)))
}

// writeError answers with the same JSON error shape as the API.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}{code, msg})
}
