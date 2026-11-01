package ingest

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
)

const (
	maxIdempotencyKeyRunes = 128
	preflightMaxAgeSeconds = "86400"
	headerNatsMsgID        = "Nats-Msg-Id"
	generatedIdemBytes     = 16 // 26 base32 characters
)

// Config holds the request limits.
type Config struct {
	MaxBodyBytes     int64
	MaxMessageChars  int
	MaxMetadataBytes int64
	PublishTimeout   time.Duration
}

// ConfigFrom takes the limits from cfg.
func ConfigFrom(cfg config.Config) Config {
	return Config{
		MaxBodyBytes:     cfg.MaxBodyBytes,
		MaxMessageChars:  cfg.MaxMessageChars,
		MaxMetadataBytes: cfg.MaxMetadataBytes,
		PublishTimeout:   cfg.PublishTimeout,
	}
}

// Deps are the handler's collaborators. The caller runs Cache.Run, Limiter.Run
// and Counters.Run.
type Deps struct {
	Cache    *KeyCache
	Limiter  *Limiter
	Counters *Counters
	JS       jetstream.JetStream
	Config   Config
	// Ready, once closed, lets requests through; nil = Cache.Ready(). Wiring passes a
	// channel that also waits for Counters.Load.
	Ready <-chan struct{}
	Log   *slog.Logger
	Now   func() time.Time // nil = time.Now
}

type handler struct{ Deps }

// NewHandler returns the ingest endpoint.
func NewHandler(d Deps) http.Handler {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.Ready == nil {
		d.Ready = d.Cache.Ready()
	}
	return &handler{d}
}

type payload struct {
	Message        json.RawMessage `json:"message"`
	Metadata       json.RawMessage `json:"metadata"`
	IdempotencyKey json.RawMessage `json:"idempotencyKey"`
	UserID         json.RawMessage `json:"userId"`
	AppVersion     json.RawMessage `json:"appVersion"`
	Platform       json.RawMessage `json:"platform"`
}

func (h *handler) reject(w http.ResponseWriter, status int, code, msg string, retryAfter int) {
	h.Log.Debug("ingest: rejected", "status", status, "code", code)
	writeError(w, status, code, msg, retryAfter)
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hd := w.Header()
	hd.Set("Access-Control-Allow-Origin", "*")
	hd.Set("Access-Control-Expose-Headers", "Retry-After")

	if r.Method == http.MethodOptions {
		hd.Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		hd.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		hd.Set("Access-Control-Max-Age", preflightMaxAgeSeconds)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		hd.Set("Allow", "POST, OPTIONS")
		h.reject(w, http.StatusMethodNotAllowed, codeMethodNotAllowed, "Use POST to submit feedback.", 0)
		return
	}

	select {
	case <-h.Ready:
	default:
		h.reject(w, http.StatusServiceUnavailable, codeQueueFull, "The service is starting. Try again shortly.", unavailableRetrySeconds)
		return
	}
	entry, ok := h.authenticate(r)
	if !ok {
		h.reject(w, http.StatusUnauthorized, codeInvalidKey, "The ingest key is missing, unknown or revoked.", 0)
		return
	}

	body, err := readBody(w, r, h.Config.MaxBodyBytes)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			h.reject(w, http.StatusRequestEntityTooLarge, codePayloadTooLarge, "The request body is too large.", 0)
		} else {
			h.reject(w, http.StatusBadRequest, codeInvalidPayload, "The request body could not be read.", 0)
		}
		return
	}

	now := h.Now()
	if ok, wait := h.Limiter.Allow(entry.KeyID, now); !ok {
		h.reject(w, http.StatusTooManyRequests, codeRateLimited, "Too many requests for this key. Slow down.", ceilSeconds(wait))
		return
	}

	v := h.Counters.TryIncr(entry.TenantID, ext.Limits{Daily: entry.DailyLimit, Monthly: entry.MonthlyLimit}, entry.BillingMonthStart, now)
	switch v.Result {
	case DailyLimited:
		h.reject(w, http.StatusTooManyRequests, codeDailyLimit, "The daily submission limit is reached.", ceilSeconds(v.RetryAfter))
		return
	case PlanLimited:
		h.reject(w, http.StatusTooManyRequests, codePlanLimit, "The plan's monthly submission limit is reached.", 0)
		return
	}

	idem, ok := h.checkPayload(body)
	if !ok {
		h.Counters.Refund(v)
		h.reject(w, http.StatusBadRequest, codeInvalidPayload, "The request body is not a valid feedback submission.", 0)
		return
	}
	if idem == "" {
		idem = generateIdem()
	}

	hdr := nats.Header{}
	hdr.Set(queue.HeaderTenant, entry.TenantID)
	hdr.Set(queue.HeaderChannel, entry.ChannelID)
	hdr.Set(queue.HeaderKey, entry.KeyID)
	hdr.Set(queue.HeaderReceived, now.UTC().Format(time.RFC3339Nano))
	hdr.Set(queue.HeaderIdem, idem)
	hdr.Set(headerNatsMsgID, entry.TenantID+":"+idem)

	// Detached from the request so a client hang-up cannot race a publish
	// that succeeds, which would refund a message that is in the queue.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), h.Config.PublishTimeout)
	defer cancel()
	ack, err := h.JS.PublishMsg(pctx, &nats.Msg{Subject: queue.SubjectIngest, Header: hdr, Data: body})
	if err != nil {
		h.Counters.Refund(v)
		if queue.IsQueueFull(err) {
			h.Log.Debug("ingest: queue full")
		} else {
			h.Log.Warn("ingest: publish failed", "err", err)
		}
		h.reject(w, http.StatusServiceUnavailable, codeQueueFull, "The service cannot accept feedback right now. Try again shortly.", unavailableRetrySeconds)
		return
	}
	if ack.Duplicate {
		h.Counters.Refund(v) // a retry of an already queued submission adds nothing
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *handler) authenticate(r *http.Request) (Entry, bool) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
	token = strings.TrimSpace(token)
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return Entry{}, false
	}
	e, ok := h.Cache.Lookup(HashKey(token))
	if !ok || e.Revoked {
		return Entry{}, false
	}
	return e, true
}

func readBody(w http.ResponseWriter, r *http.Request, max int64) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, max))
}

// checkPayload validates the body shape and returns the client's idempotency
// key ("" when absent).
func (h *handler) checkPayload(body []byte) (string, bool) {
	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		return "", false
	}
	var msg string
	if !isNonNull(p.Message) || json.Unmarshal(p.Message, &msg) != nil ||
		strings.TrimSpace(msg) == "" || utf8.RuneCountInString(msg) > h.Config.MaxMessageChars {
		return "", false
	}
	if isNonNull(p.Metadata) {
		if int64(len(p.Metadata)) > h.Config.MaxMetadataBytes || strings.TrimLeft(string(p.Metadata), " \t\r\n")[0] != '{' {
			return "", false
		}
	}
	for _, f := range []json.RawMessage{p.UserID, p.AppVersion, p.Platform} {
		var s string
		if isNonNull(f) && json.Unmarshal(f, &s) != nil {
			return "", false
		}
	}
	var idem string
	if isNonNull(p.IdempotencyKey) {
		// Control characters would corrupt the NATS header carrying the key.
		if json.Unmarshal(p.IdempotencyKey, &idem) != nil || idem == "" ||
			utf8.RuneCountInString(idem) > maxIdempotencyKeyRunes || strings.IndexFunc(idem, unicode.IsControl) >= 0 {
			return "", false
		}
	}
	return idem, true
}

func isNonNull(raw json.RawMessage) bool {
	return len(raw) > 0 && string(raw) != "null"
}

func generateIdem() string {
	var b [generatedIdemBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("ingest: crypto/rand failed: " + err.Error())
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

// ceilSeconds rounds d up to whole seconds, at least 1.
func ceilSeconds(d time.Duration) int {
	s := int((d + time.Second - 1) / time.Second)
	if s < 1 {
		return 1
	}
	return s
}
