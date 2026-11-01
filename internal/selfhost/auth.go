package selfhost

import (
	"net/http"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/api"
	"github.com/njkuhne/feedbaxter/internal/service"
)

var _ ext.Authenticator = (*Auth)(nil)

// Auth is the self-host ext.Authenticator: email and password, sessions in
// cookies. The role rules all live in the service.
type Auth struct {
	Svc      *service.Service
	Sessions ext.SessionStore
	Tenants  ext.TenantResolver
}

// Principal resolves the session cookie of r.
func (a *Auth) Principal(r *http.Request) (ext.Principal, bool) { return a.Sessions.Principal(r) }

// Mount registers the account routes on the mux that serves /api/.
func (a *Auth) Mount(mux *http.ServeMux, deps ext.Deps) {
	log := deps.Logger
	// fail answers err; handlers that need a signed-in caller call need first.
	fail := func(w http.ResponseWriter, err error) { api.WriteError(w, log, err) }
	need := func(w http.ResponseWriter, r *http.Request) (ext.Principal, bool) {
		p, err := api.RequirePrincipal(r)
		if err != nil {
			fail(w, err)
		}
		return p, err == nil
	}

	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := api.DecodeJSON(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		tn, err := a.Tenants.Default(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		u, err := a.Svc.Login(r.Context(), tn.ID, in.Email, in.Password)
		if err != nil {
			fail(w, err)
			return
		}
		if err := a.Sessions.Start(w, r, u.ID, u.TenantID); err != nil {
			fail(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, struct {
			User api.MeJSON `json:"user"`
		}{api.NewMeJSON(u)})
	})

	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if err := a.Sessions.End(w, r); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/auth/password", func(w http.ResponseWriter, r *http.Request) {
		p, ok := need(w, r)
		if !ok {
			return
		}
		var in struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		if err := api.DecodeJSON(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		keep := ""
		if c, err := r.Cookie(api.SessionCookieName); err == nil {
			keep = c.Value
		}
		if err := a.Svc.ChangePassword(r.Context(), p, in.CurrentPassword, in.NewPassword, keep); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		p, ok := need(w, r)
		if !ok {
			return
		}
		var in struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := api.DecodeJSON(w, r, &in); err != nil {
			fail(w, err)
			return
		}
		u, temp, err := a.Svc.CreateUser(r.Context(), p, in.Email, in.Role)
		if err != nil {
			fail(w, err)
			return
		}
		api.WriteJSON(w, http.StatusCreated, struct {
			User              api.UserJSON `json:"user"`
			TemporaryPassword string       `json:"temporaryPassword"`
		}{api.NewUserJSON(u), temp})
	})

	mux.HandleFunc("POST /api/users/{id}/reset-password", func(w http.ResponseWriter, r *http.Request) {
		p, ok := need(w, r)
		if !ok {
			return
		}
		temp, err := a.Svc.ResetPassword(r.Context(), p, r.PathValue("id"))
		if err != nil {
			fail(w, err)
			return
		}
		api.WriteJSON(w, http.StatusOK, struct {
			TemporaryPassword string `json:"temporaryPassword"`
		}{temp})
	})
}
