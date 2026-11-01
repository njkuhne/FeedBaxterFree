package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/service"
)

func parseTime(q, name string) (*time.Time, error) {
	s := q
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, invalidRequest("Use an RFC 3339 date for " + name + ", for example 2026-03-01T00:00:00Z.")
	}
	return &t, nil
}

// filterFrom reads the filter parameters shared by list, export and stats.
func filterFrom(r *http.Request) (service.Filter, error) {
	q := r.URL.Query()
	f := service.Filter{
		ChannelID: q.Get("channel"), Tag: q.Get("tag"), UserID: q.Get("userId"), Q: q.Get("q"),
	}
	for _, v := range q["status"] {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				f.Statuses = append(f.Statuses, s)
			}
		}
	}
	var err error
	if f.From, err = parseTime(q.Get("from"), "from"); err != nil {
		return f, err
	}
	if f.To, err = parseTime(q.Get("to"), "to"); err != nil {
		return f, err
	}
	return f, nil
}

func (a *api) me(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	u, err := a.svc.Me(r.Context(), p)
	if err != nil {
		return err
	}
	if p.Via == "ui" {
		refreshCookie(w, r)
	}
	WriteJSON(w, http.StatusOK, NewMeJSON(u))
	return nil
}

func (a *api) listFeedback(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	f, err := filterFrom(r)
	if err != nil {
		return err
	}
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if limit, err = strconv.Atoi(s); err != nil {
			return invalidRequest("Limit must be a whole number between 1 and 200.")
		}
		if limit == 0 {
			return invalidRequest("Limit must be between 1 and 200.")
		}
	}
	pg, err := a.svc.ListFeedback(r.Context(), p, f, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		return err
	}
	items := make([]feedbackJSON, len(pg.Items))
	for i, it := range pg.Items {
		items[i] = newFeedbackJSON(it)
	}
	var next *string
	if pg.NextCursor != "" {
		next = &pg.NextCursor
	}
	WriteJSON(w, http.StatusOK, struct {
		Items      []feedbackJSON `json:"items"`
		NextCursor *string        `json:"nextCursor"`
	}{items, next})
	return nil
}

func (a *api) getFeedback(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	d, err := a.svc.GetFeedback(r.Context(), p, r.PathValue("id"))
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, newDetailJSON(d))
	return nil
}

func (a *api) patchFeedback(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in struct {
		Status string `json:"status"`
	}
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	id := r.PathValue("id")
	res, err := a.svc.SetStatus(r.Context(), p, []string{id}, in.Status)
	if err != nil {
		return err
	}
	if len(res.NotFound) > 0 {
		return &service.Error{Kind: service.KindNotFound, Msg: "Feedback not found."}
	}
	d, err := a.svc.GetFeedback(r.Context(), p, id)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, newFeedbackJSON(d.Feedback))
	return nil
}

func (a *api) deleteFeedback(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	if err := a.svc.DeleteFeedback(r.Context(), p, r.PathValue("id")); err != nil {
		return err
	}
	noContent(w)
	return nil
}

type bulkOut struct {
	Updated  int      `json:"updated"`
	NotFound []string `json:"notFound"`
}

func writeBulk(w http.ResponseWriter, res service.BulkResult) {
	nf := res.NotFound
	if nf == nil {
		nf = []string{}
	}
	WriteJSON(w, http.StatusOK, bulkOut{Updated: res.Updated, NotFound: nf})
}

func (a *api) bulkStatus(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in struct {
		IDs    []string `json:"ids"`
		Status string   `json:"status"`
	}
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	res, err := a.svc.SetStatus(r.Context(), p, in.IDs, in.Status)
	if err != nil {
		return err
	}
	writeBulk(w, res)
	return nil
}

// bulkTags serves add-tags and remove-tags.
func (a *api) bulkTags(op func(context.Context, ext.Principal, []string, []string) (service.BulkResult, error)) handlerFunc {
	return func(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
		var in struct {
			IDs  []string `json:"ids"`
			Tags []string `json:"tags"`
		}
		if err := DecodeJSON(w, r, &in); err != nil {
			return err
		}
		res, err := op(r.Context(), p, in.IDs, in.Tags)
		if err != nil {
			return err
		}
		writeBulk(w, res)
		return nil
	}
}

func (a *api) deleteByUser(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	var in struct {
		UserID string `json:"userId"`
	}
	if err := DecodeJSON(w, r, &in); err != nil {
		return err
	}
	n, err := a.svc.DeleteUserFeedback(r.Context(), p, in.UserID)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, struct {
		Deleted int64 `json:"deleted"`
	}{n})
	return nil
}

func (a *api) listTags(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	ts, err := a.svc.ListTags(r.Context(), p)
	if err != nil {
		return err
	}
	type tag struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	items := make([]tag, len(ts))
	for i, t := range ts {
		items[i] = tag{t.ID, t.Name, t.Count}
	}
	WriteJSON(w, http.StatusOK, struct {
		Items []tag `json:"items"`
	}{items})
	return nil
}

func (a *api) stats(w http.ResponseWriter, r *http.Request, p ext.Principal) error {
	q := r.URL.Query()
	from, err := parseTime(q.Get("from"), "from")
	if err != nil {
		return err
	}
	to, err := parseTime(q.Get("to"), "to")
	if err != nil {
		return err
	}
	st, err := a.svc.Stats(r.Context(), p, from, to)
	if err != nil {
		return err
	}
	type chCount struct {
		ChannelID string  `json:"channelId"`
		Name      *string `json:"name"`
		Count     int64   `json:"count"`
	}
	type tagCount struct {
		Name  string `json:"name"`
		Count int64  `json:"count"`
	}
	out := struct {
		Total     int64            `json:"total"`
		ByStatus  map[string]int64 `json:"byStatus"`
		ByChannel []chCount        `json:"byChannel"`
		ByTag     []tagCount       `json:"byTag"`
	}{Total: st.Total, ByStatus: st.ByStatus, ByChannel: []chCount{}, ByTag: []tagCount{}}
	if out.ByStatus == nil {
		out.ByStatus = map[string]int64{}
	}
	for _, c := range st.ByChannel {
		out.ByChannel = append(out.ByChannel, chCount{c.ChannelID, c.Name, c.Count})
	}
	for _, t := range st.ByTag {
		out.ByTag = append(out.ByTag, tagCount{t.Name, t.Count})
	}
	WriteJSON(w, http.StatusOK, out)
	return nil
}
