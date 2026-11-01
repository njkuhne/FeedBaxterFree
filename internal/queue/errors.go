package queue

import (
	"errors"
	"strings"

	"github.com/nats-io/nats.go/jetstream"
)

// errCodeStreamStoreFailed is the server's JSStreamStoreFailedF (nats-server
// jetstream_errors_generated.go, code 503). DiscardNew limit hits arrive as
// this error with the store error text as description; nats.go has no constant.
const errCodeStreamStoreFailed jetstream.ErrorCode = 10077

// Texts of the server's ErrMaxMsgs / ErrMaxBytes (server/store.go).
const (
	descMaxMsgs  = "maximum messages exceeded"
	descMaxBytes = "maximum bytes exceeded"
)

// IsQueueFull reports whether err is a stream refusing a message under
// DiscardNew because MaxMsgs or MaxBytes is reached.
func IsQueueFull(err error) bool {
	var api *jetstream.APIError
	if !errors.As(err, &api) || api.ErrorCode != errCodeStreamStoreFailed {
		return false
	}
	d := strings.TrimSpace(api.Description)
	return d == descMaxMsgs || d == descMaxBytes
}

// IsWrongLastSequence reports a compare-and-swap conflict.
func IsWrongLastSequence(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyRevisionMismatch) {
		return true
	}
	var api *jetstream.APIError
	if errors.As(err, &api) {
		return api.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence ||
			api.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequenceConstant
	}
	return false
}
