// Package api is the JSON HTTP layer of the admin inbox. It is thin: it
// parses requests, calls internal/service (which enforces every role rule) and
// renders the answer. Nothing here decides who may do what.
package api

import (
	"log/slog"
	"net/http"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/service"
)

// Config is what New needs.
type Config struct {
	Svc  *service.Service
	Auth ext.Authenticator // may be nil: then nobody can sign in
	Deps ext.Deps
	Log  *slog.Logger // nil = slog.Default()
}

type api struct {
	svc  *service.Service
	auth ext.Authenticator
	log  *slog.Logger
	mux  *http.ServeMux
}

// New returns the handler for /api/…; the caller mounts it at "/api/". It
// calls cfg.Auth.Mount on an inner mux, adds the routes below and wraps the
// whole in the CSRF check, principal resolution and the must-change gate.
func New(cfg Config) http.Handler {
	a := &api{svc: cfg.Svc, auth: cfg.Auth, log: cfg.Log, mux: http.NewServeMux()}
	if a.log == nil {
		a.log = slog.Default()
	}
	if cfg.Auth != nil {
		cfg.Auth.Mount(a.mux, cfg.Deps)
	}
	a.routes()
	return a
}

// handlerFunc is a route handler that has a principal. It answers success
// itself and returns any error for WriteError.
type handlerFunc func(w http.ResponseWriter, r *http.Request, p ext.Principal) error

// h wraps f: 401 without a principal, WriteError on a returned error.
func (a *api) h(f handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := RequirePrincipal(r)
		if err == nil {
			err = f(w, r, p)
		}
		if err != nil {
			WriteError(w, a.log, err)
		}
	}
}

// RequirePrincipal returns the principal the middleware resolved, or an
// unauthenticated error for WriteError.
func RequirePrincipal(r *http.Request) (ext.Principal, error) {
	p, ok := ext.PrincipalFrom(r.Context())
	if !ok {
		return ext.Principal{}, &service.Error{Kind: service.KindUnauthenticated, Msg: "Sign in to continue."}
	}
	return p, nil
}

func (a *api) routes() {
	m := a.mux
	m.HandleFunc("GET /api/me", a.h(a.me))

	m.HandleFunc("GET /api/users", a.h(a.listUsers))
	m.HandleFunc("PATCH /api/users/{id}", a.h(a.setRole))
	m.HandleFunc("DELETE /api/users/{id}", a.h(a.deleteUser))

	m.HandleFunc("GET /api/tokens", a.h(a.listTokens))
	m.HandleFunc("POST /api/tokens", a.h(a.createToken))
	m.HandleFunc("DELETE /api/tokens/{id}", a.h(a.deleteToken))

	m.HandleFunc("GET /api/channels", a.h(a.listChannels))
	m.HandleFunc("POST /api/channels", a.h(a.createChannel))
	m.HandleFunc("PATCH /api/channels/{id}", a.h(a.renameChannel))
	m.HandleFunc("DELETE /api/channels/{id}", a.h(a.deleteChannel))

	m.HandleFunc("GET /api/keys", a.h(a.listKeys))
	m.HandleFunc("POST /api/channels/{id}/keys", a.h(a.createKey))
	m.HandleFunc("POST /api/keys/{id}/revoke", a.h(a.revokeKey))

	m.HandleFunc("GET /api/feedback", a.h(a.listFeedback))
	m.HandleFunc("GET /api/feedback/export", a.h(a.exportFeedback))
	m.HandleFunc("GET /api/feedback/{id}", a.h(a.getFeedback))
	m.HandleFunc("PATCH /api/feedback/{id}", a.h(a.patchFeedback))
	m.HandleFunc("DELETE /api/feedback/{id}", a.h(a.deleteFeedback))
	m.HandleFunc("POST /api/feedback/status", a.h(a.bulkStatus))
	m.HandleFunc("POST /api/feedback/add-tags", a.h(a.bulkTags(a.svc.AddTags)))
	m.HandleFunc("POST /api/feedback/remove-tags", a.h(a.bulkTags(a.svc.RemoveTags)))
	m.HandleFunc("POST /api/feedback/delete-by-user", a.h(a.deleteByUser))

	m.HandleFunc("GET /api/tags", a.h(a.listTags))
	m.HandleFunc("GET /api/stats", a.h(a.stats))
	m.HandleFunc("GET /api/ops", a.h(a.ops))
}
