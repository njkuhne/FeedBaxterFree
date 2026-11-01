package api

import (
	"net/http"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/service"
)

func (a *api) listUsers(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	us, err := a.svc.ListUsers(r.Context(), p)
	if err != nil {
		return err
	}
	items := make([]UserJSON, len(us))
	for i, u := range us {
		items[i] = NewUserJSON(u)
	}
	WriteJSON(w, http.StatusOK, struct {
		Items []UserJSON `json:"items"`
	}{items})
	return nil
}

func (a *api) setRole(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in struct {
		Role string `json:"role"`
	}
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := a.svc.SetRole(r.Context(), p, id, in.Role); err != nil {
		return err
	}
	us, err := a.svc.ListUsers(r.Context(), p)
	if err != nil {
		return err
	}
	for _, u := range us {
		if u.ID == id {
			WriteJSON(w, http.StatusOK, NewUserJSON(u))
			return nil
		}
	}
	return &service.Error{Kind: service.KindNotFound, Msg: "User not found."}
}

func (a *api) deleteUser(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	if err := a.svc.DeleteUser(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	noContent(w)
	return nil
}

func (a *api) listTokens(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	ts, err := a.svc.ListTokens(r.Context(), p)
	if err != nil {
		return err
	}
	items := make([]tokenJSON, len(ts))
	for i, t := range ts {
		items[i] = newTokenJSON(t)
	}
	WriteJSON(w, http.StatusOK, struct {
		Items []tokenJSON `json:"items"`
	}{items})
	return nil
}

func (a *api) createToken(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	t, secret, err := a.svc.CreateToken(r.Context(), p, in.Name)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusCreated, struct {
		Token  tokenJSON `json:"token"`
		Secret string    `json:"secret"`
	}{newTokenJSON(t), secret})
	return nil
}

func (a *api) deleteToken(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	if err := a.svc.RevokeToken(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	noContent(w)
	return nil
}

func (a *api) listChannels(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	cs, err := a.svc.ListChannels(r.Context(), p)
	if err != nil {
		return err
	}
	items := make([]channelJSON, len(cs))
	for i, c := range cs {
		items[i] = newChannelJSON(c)
	}
	WriteJSON(w, http.StatusOK, struct {
		Items []channelJSON `json:"items"`
	}{items})
	return nil
}

type nameBody struct {
	Name string `json:"name"`
}

func (a *api) createChannel(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in nameBody
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	c, err := a.svc.CreateChannel(r.Context(), p, in.Name)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusCreated, newChannelJSON(c))
	return nil
}

func (a *api) renameChannel(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in nameBody
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	if err := a.svc.RenameChannel(r.Context(), p, id, in.Name); err != nil {
		return err
	}
	cs, err := a.svc.ListChannels(r.Context(), p)
	if err != nil {
		return err
	}
	for _, c := range cs {
		if c.ID == id {
			WriteJSON(w, http.StatusOK, newChannelJSON(c))
			return nil
		}
	}
	return &service.Error{Kind: service.KindNotFound, Msg: "Channel not found."}
}

func (a *api) deleteChannel(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	if err := a.svc.DeleteChannel(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	noContent(w)
	return nil
}

func (a *api) listKeys(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	ks, err := a.svc.ListKeys(r.Context(), p, r.URL.Query().Get("channel"))
	if err != nil {
		return err
	}
	items := make([]keyJSON, len(ks))
	for i, k := range ks {
		items[i] = newKeyJSON(k)
	}
	WriteJSON(w, http.StatusOK, struct {
		Items []keyJSON `json:"items"`
	}{items})
	return nil
}

func (a *api) createKey(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	k, secret, err := a.svc.CreateKey(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusCreated, struct {
		Key    keyJSON `json:"key"`
		Secret string  `json:"secret"`
	}{newKeyJSON(k), secret})
	return nil
}

func (a *api) revokeKey(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	k, err := a.svc.RevokeKey(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, newKeyJSON(k))
	return nil
}

func (a *api) ops(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	o, err := a.svc.Ops(r.Context(), p)
	if err != nil {
		return err
	}
	type queueJSON struct {
		Depth            uint64  `json:"depth"`
		OldestAgeSeconds float64 `json:"oldestAgeSeconds"`
		DLQ              uint64  `json:"dlq"`
	}
	type countersJSON struct {
		Day      int64  `json:"day"`
		Month    int64  `json:"month"`
		DayKey   string `json:"dayKey"`
		MonthKey string `json:"monthKey"`
	}
	WriteJSON(w, http.StatusOK, struct {
		Queue       queueJSON    `json:"queue"`
		Counters    countersJSON `json:"counters"`
		DBSizeBytes int64        `json:"dbSizeBytes"`
		Version     string       `json:"version"`
		Mode        string       `json:"mode"`
		Dialect     string       `json:"dialect"`
	}{
		queueJSON{o.Queue.Depth, o.Queue.OldestAgeSeconds, o.Queue.DLQ},
		countersJSON{o.Counters.Day, o.Counters.Month, o.Counters.DayKey, o.Counters.MonthKey},
		o.DBSizeBytes, o.Version, o.Mode, o.Dialect,
	})
	return nil
}
