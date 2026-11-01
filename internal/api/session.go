package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/service"
	"github.com/njkuhne/feedbaxter/internal/store"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "fb_session"

type sessions struct{ svc *service.Service }

// NewSessionStore returns the one cookie implementation, backed by the
// service's session functions.
func NewSessionStore(svc *service.Service) ext.SessionStore { return &sessions{svc: svc} }

func (s *sessions) log() *slog.Logger {
	if s.svc.Log != nil {
		return s.svc.Log
	}
	return slog.Default()
}

// secure reports whether the client reached us over TLS, directly or through
// a proxy that says so.
func secure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func cookieFor(r *http.Request, value string) *http.Cookie {
	return &http.Cookie{
		Name: SessionCookieName, Value: value, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure(r),
	}
}

// Start replaces any session the request already carries with a new one.
func (s *sessions) Start(w http.ResponseWriter, r *http.Request, userID, tenantID string) error {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		if err := s.svc.DeleteSession(r.Context(), c.Value); err != nil {
			s.log().Warn("api: delete replaced session", "err", err)
		}
	}
	token, expires, err := s.svc.CreateSession(r.Context(), store.User{ID: userID, TenantID: tenantID})
	if err != nil {
		return err
	}
	c := cookieFor(r, token)
	c.Expires = expires
	c.MaxAge = int(service.SessionTTL.Seconds())
	http.SetCookie(w, c)
	return nil
}

// Principal resolves the cookie of r. A missing, unknown or expired session
// is simply "no principal"; a database failure is logged and treated the same.
func (s *sessions) Principal(r *http.Request) (ext.Principal, bool) {
	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return ext.Principal{}, false
	}
	p, err := s.svc.ResolveSession(r.Context(), c.Value)
	if err != nil {
		if service.KindOf(err) == 0 {
			s.log().Error("api: resolve session", "err", err)
		}
		return ext.Principal{}, false
	}
	return p, true
}

// refreshCookie re-issues the request's session cookie with a fresh 30-day
// expiry. The server-side expiry slides in ResolveSession, but Principal has no
// ResponseWriter, so GET /api/me (called by the UI on every load) renews the
// browser side; without it an active user would lose the cookie after 30 days.
func refreshCookie(w http.ResponseWriter, r *http.Request) {
	old, err := r.Cookie(SessionCookieName)
	if err != nil || old.Value == "" {
		return
	}
	c := cookieFor(r, old.Value)
	c.Expires = time.Now().Add(service.SessionTTL)
	c.MaxAge = int(service.SessionTTL.Seconds())
	http.SetCookie(w, c)
}

// End deletes the session of r, if any, and clears the cookie.
func (s *sessions) End(w http.ResponseWriter, r *http.Request) error {
	var err error
	if c, cerr := r.Cookie(SessionCookieName); cerr == nil {
		err = s.svc.DeleteSession(r.Context(), c.Value)
	}
	c := cookieFor(r, "")
	c.MaxAge = -1
	http.SetCookie(w, c)
	return err
}
