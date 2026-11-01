package api

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/njkuhne/feedbaxterfree/internal/service"
)

// Error codes that are not service kinds.
const (
	codeInvalidRequest   = "invalid_request"
	codeUnauthenticated  = "unauthenticated"
	codeForbidden        = "forbidden"
	codeNotFound         = "not_found"
	codeConflict         = "conflict"
	codeInternal         = "internal"
	codeCSRF             = "csrf"
	codePasswordChange   = "password_change_required"
	codePayloadTooLarge  = "payload_too_large"
	codeMethodNotAllowed = "method_not_allowed"
)

// errorBody is the one error shape of the API (same as the ingest API).
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// apiError is an error with an explicit status and code, for conditions that
// are not service kinds.
type apiError struct {
	status    int
	code, msg string
}

func (e *apiError) Error() string { return e.msg }

func invalidRequest(msg string) error {
	return &service.Error{Kind: service.KindInvalid, Msg: msg}
}

func writeCode(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, errorBody{Error: code, Message: message})
}

// WriteError answers err in the standard error body. A service error maps by
// Kind; anything else is logged at error level (never with the request body)
// and answered with a generic 500.
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		writeCode(w, ae.status, ae.code, ae.msg)
		return
	}
	var se *service.Error
	if errors.As(err, &se) {
		switch se.Kind {
		case service.KindInvalid:
			writeCode(w, http.StatusBadRequest, codeInvalidRequest, se.Msg)
		case service.KindUnauthenticated:
			writeCode(w, http.StatusUnauthorized, codeUnauthenticated, se.Msg)
		case service.KindForbidden:
			writeCode(w, http.StatusForbidden, codeForbidden, se.Msg)
		case service.KindNotFound:
			writeCode(w, http.StatusNotFound, codeNotFound, se.Msg)
		case service.KindConflict:
			writeCode(w, http.StatusConflict, codeConflict, se.Msg)
		default:
			se = nil
		}
		if se != nil {
			return
		}
	}
	if log == nil {
		log = slog.Default()
	}
	log.Error("api: internal error", "err", err)
	writeCode(w, http.StatusInternalServerError, codeInternal, "Something went wrong.")
}
