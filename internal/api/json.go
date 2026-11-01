package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/njkuhne/feedbaxter/internal/service"
	"github.com/njkuhne/feedbaxter/internal/store"
)

const (
	maxBodyBytes = 1 << 20
	timeLayout   = "2006-01-02T15:04:05.000Z"
)

// WriteJSON answers status with v as JSON.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, `{"error":"internal","message":"Something went wrong."}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func noContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// DecodeJSON reads the request body into dst: at most 1 MiB, no unknown
// fields, exactly one value. The returned error is for WriteError.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		if err = dec.Decode(&struct{}{}); err == io.EOF {
			return nil
		}
		if err == nil {
			err = errors.New("trailing data")
		}
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return &apiError{status: http.StatusRequestEntityTooLarge, code: codePayloadTooLarge, msg: "The request body is too large."}
	}
	return invalidRequest("The request body is not valid JSON for this endpoint.")
}

// fmtMs renders Unix milliseconds as RFC 3339 UTC with milliseconds.
func fmtMs(ms int64) string { return time.UnixMilli(ms).UTC().Format(timeLayout) }

func fmtMsPtr(ms *int64) *string {
	if ms == nil {
		return nil
	}
	s := fmtMs(*ms)
	return &s
}

// MeJSON is the signed-in user as /api/me and the login response show it.
type MeJSON struct {
	ID                 string `json:"id"`
	Email              string `json:"email"`
	Role               string `json:"role"`
	MustChangePassword bool   `json:"mustChangePassword"`
}

// NewMeJSON converts a user row.
func NewMeJSON(u store.User) MeJSON {
	return MeJSON{ID: u.ID, Email: u.Email, Role: u.Role, MustChangePassword: u.MustChangePassword}
}

// UserJSON is a user as the people list shows it.
type UserJSON struct {
	MeJSON
	CreatedAt string `json:"createdAt"`
}

// NewUserJSON converts a user row.
func NewUserJSON(u store.User) UserJSON {
	return UserJSON{MeJSON: NewMeJSON(u), CreatedAt: fmtMs(u.CreatedAt)}
}

type tokenJSON struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	CreatedAt  string  `json:"createdAt"`
	LastUsedAt *string `json:"lastUsedAt"`
}

func newTokenJSON(t store.Token) tokenJSON {
	return tokenJSON{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: fmtMs(t.CreatedAt), LastUsedAt: fmtMsPtr(t.LastUsedAt)}
}

type channelJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

func newChannelJSON(c store.Channel) channelJSON {
	return channelJSON{ID: c.ID, Name: c.Name, CreatedAt: fmtMs(c.CreatedAt)}
}

type keyJSON struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channelId"`
	Prefix    string  `json:"prefix"`
	CreatedAt string  `json:"createdAt"`
	RevokedAt *string `json:"revokedAt"`
}

func newKeyJSON(k store.IngestKey) keyJSON {
	return keyJSON{ID: k.ID, ChannelID: k.ChannelID, Prefix: k.Prefix, CreatedAt: fmtMs(k.CreatedAt), RevokedAt: fmtMsPtr(k.RevokedAt)}
}

type feedbackJSON struct {
	ID          string          `json:"id"`
	ChannelID   string          `json:"channelId"`
	ChannelName *string         `json:"channelName"`
	Message     string          `json:"message"`
	UserID      *string         `json:"userId"`
	AppVersion  *string         `json:"appVersion"`
	Platform    *string         `json:"platform"`
	Metadata    json.RawMessage `json:"metadata"`
	Status      string          `json:"status"`
	ReceivedAt  string          `json:"receivedAt"`
	UpdatedAt   string          `json:"updatedAt"`
	UpdatedBy   *string         `json:"updatedBy"`
	Tags        []string        `json:"tags"`
}

func newFeedbackJSON(f store.Feedback) feedbackJSON {
	tags := f.Tags
	if tags == nil {
		tags = []string{}
	}
	meta := f.Metadata
	if len(meta) == 0 {
		meta = nil
	}
	return feedbackJSON{
		ID: f.ID, ChannelID: f.ChannelID, ChannelName: f.ChannelName, Message: f.Message,
		UserID: f.UserID, AppVersion: f.AppVersion, Platform: f.Platform, Metadata: meta,
		Status: f.Status, ReceivedAt: fmtMs(f.ReceivedAt), UpdatedAt: fmtMs(f.UpdatedAt),
		UpdatedBy: f.UpdatedBy, Tags: tags,
	}
}

type historyJSON struct {
	From      string  `json:"from"`
	To        string  `json:"to"`
	UserID    string  `json:"userId"`
	UserEmail *string `json:"userEmail"`
	Via       string  `json:"via"`
	At        string  `json:"at"`
}

type detailJSON struct {
	feedbackJSON
	History []historyJSON `json:"history"`
}

func newDetailJSON(d service.Detail) detailJSON {
	h := make([]historyJSON, len(d.History))
	for i, c := range d.History {
		h[i] = historyJSON{From: c.From, To: c.To, UserID: c.UserID, UserEmail: c.UserEmail, Via: c.Via, At: fmtMs(c.ChangedAt)}
	}
	return detailJSON{feedbackJSON: newFeedbackJSON(d.Feedback), History: h}
}
