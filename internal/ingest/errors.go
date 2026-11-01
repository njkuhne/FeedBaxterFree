package ingest

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Error codes in the response body (ARCHITECTURE §4).
const (
	codeMethodNotAllowed = "method_not_allowed"
	codeInvalidKey       = "invalid_key"
	codePayloadTooLarge  = "payload_too_large"
	codeInvalidPayload   = "invalid_payload"
	codeRateLimited      = "rate_limited"
	codeDailyLimit       = "daily_limit"
	codePlanLimit        = "plan_limit"
	codeQueueFull        = "queue_full"
)

// unavailableRetrySeconds is the Retry-After sent with 503 responses.
const unavailableRetrySeconds = 5

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// writeError sends the standard error body. retryAfter <= 0 sends no Retry-After.
func writeError(w http.ResponseWriter, status int, code, message string, retryAfter int) {
	h := w.Header()
	h.Set("Content-Type", "application/json")
	if retryAfter > 0 {
		h.Set("Retry-After", strconv.Itoa(retryAfter))
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Error: code, Message: message})
}
