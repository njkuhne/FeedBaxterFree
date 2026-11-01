package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// Clamp limits for the optional body fields (SPEC "Processors").
const (
	maxUserIDChars     = 256
	maxAppVersionChars = 64
	maxPlatformChars   = 32
)

// Normalise turns one message into a row. Any error means the message can
// never be stored and belongs in the DLQ.
func Normalise(h nats.Header, body []byte, cfg Config) (store.FeedbackRow, error) {
	var r store.FeedbackRow
	r.TenantID = h.Get(queue.HeaderTenant)
	r.ChannelID = h.Get(queue.HeaderChannel)
	r.IdempotencyKey = h.Get(queue.HeaderIdem)
	for name, v := range map[string]string{
		queue.HeaderTenant: r.TenantID, queue.HeaderChannel: r.ChannelID, queue.HeaderIdem: r.IdempotencyKey,
	} {
		if v == "" {
			return r, fmt.Errorf("missing header %s", name)
		}
	}
	recv := h.Get(queue.HeaderReceived)
	if recv == "" {
		return r, fmt.Errorf("missing header %s", queue.HeaderReceived)
	}
	t, err := time.Parse(time.RFC3339Nano, recv)
	if err != nil {
		return r, fmt.Errorf("bad header %s: %w", queue.HeaderReceived, err)
	}
	r.ReceivedAt = t.UnixMilli()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return r, fmt.Errorf("body is not valid JSON: %w", err)
	}
	if raw == nil {
		return r, errors.New("body is not a JSON object")
	}

	msg, ok, err := stringField(raw, "message")
	if err != nil {
		return r, err
	}
	msg = strings.TrimSpace(msg)
	if !ok || msg == "" {
		return r, errors.New("message is missing or empty")
	}
	r.Message = clamp(msg, cfg.MaxMessageChars)

	if r.UserID, err = optString(raw, "userId", maxUserIDChars, false); err != nil {
		return r, err
	}
	if r.AppVersion, err = optString(raw, "appVersion", maxAppVersionChars, false); err != nil {
		return r, err
	}
	if r.Platform, err = optString(raw, "platform", maxPlatformChars, true); err != nil {
		return r, err
	}

	if m, ok := raw["metadata"]; ok && string(m) != "null" {
		if len(m) == 0 || m[0] != '{' {
			return r, errors.New("metadata is not a JSON object")
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, m); err != nil {
			return r, fmt.Errorf("metadata: %w", err)
		}
		// Postgres jsonb cannot hold \u0000.
		if bytes.Contains(buf.Bytes(), []byte(`\u0000`)) {
			if err := stripMetadataNUL(&buf); err != nil {
				return r, fmt.Errorf("metadata: %w", err)
			}
		}
		if int64(buf.Len()) > cfg.MaxMetadataBytes {
			return r, fmt.Errorf("metadata is %d bytes, limit %d", buf.Len(), cfg.MaxMetadataBytes)
		}
		r.Metadata = buf.Bytes()
	}

	r.ID = store.NewID(store.PrefixFeedback)
	return r, nil
}

// stringField reads a string-typed field; ok is false when absent or null.
func stringField(raw map[string]json.RawMessage, key string) (s string, ok bool, err error) {
	v, present := raw[key]
	if !present || string(v) == "null" {
		return "", false, nil
	}
	if err := json.Unmarshal(v, &s); err != nil {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	// Postgres text cannot hold NUL.
	return stripNUL(s), true, nil
}

func optString(raw map[string]json.RawMessage, key string, limit int, lower bool) (*string, error) {
	s, ok, err := stringField(raw, key)
	if err != nil || !ok {
		return nil, err
	}
	s = strings.TrimSpace(s)
	if lower {
		s = strings.ToLower(s)
	}
	s = clamp(s, limit)
	if s == "" {
		return nil, nil
	}
	return &s, nil
}

// clamp keeps at most n runes of s, never splitting one.
func clamp(s string, n int) string {
	if n < 0 {
		n = 0
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i]
		}
		count++
	}
	return s
}

func stripNUL(s string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.ReplaceAll(s, "\x00", "")
}

// stripMetadataNUL removes NUL from every key and string value and re-encodes buf compactly.
func stripMetadataNUL(buf *bytes.Buffer) error {
	dec := json.NewDecoder(bytes.NewReader(buf.Bytes()))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(stripValue(v)); err != nil {
		return err
	}
	buf.Reset()
	buf.Write(bytes.TrimSuffix(out.Bytes(), []byte("\n")))
	return nil
}

func stripValue(v any) any {
	switch x := v.(type) {
	case string:
		return stripNUL(x)
	case []any:
		for i := range x {
			x[i] = stripValue(x[i])
		}
		return x
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, e := range x {
			m[stripNUL(k)] = stripValue(e)
		}
		return m
	}
	return v
}
