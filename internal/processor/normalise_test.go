package processor_test

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"

	"github.com/njkuhne/feedbaxter/internal/processor"
	"github.com/njkuhne/feedbaxter/internal/queue"
)

func testCfg() processor.Config {
	return processor.Config{
		Concurrency: 1, BatchSize: 100, FetchMaxWait: 100 * time.Millisecond, MaxDeliver: 5,
		MaxMessageChars: 5000, MaxMetadataBytes: 4096,
		RetentionWindow: 24 * time.Hour, NakDelay: 10 * time.Millisecond,
	}
}

func hdr(tenant, channel, idem string, received time.Time) nats.Header {
	h := nats.Header{}
	h.Set(queue.HeaderTenant, tenant)
	h.Set(queue.HeaderChannel, channel)
	h.Set(queue.HeaderKey, "ky_x")
	h.Set(queue.HeaderIdem, idem)
	h.Set(queue.HeaderReceived, received.UTC().Format(time.RFC3339Nano))
	return h
}

func TestNormaliseFull(t *testing.T) {
	recv := time.Date(2026, 5, 6, 7, 8, 9, 123456789, time.UTC)
	h := hdr("tn_1", "ch_1", "idem-1", recv)
	body := `{"message":"hello","userId":" u-1 ","appVersion":"1.2.3","platform":"iOS",
		"metadata":{ "a" : 1 },"idempotencyKey":"ignored"}`
	r, err := processor.Normalise(h, []byte(body), testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(r.ID, "fb_") || r.TenantID != "tn_1" || r.ChannelID != "ch_1" ||
		r.Message != "hello" || r.IdempotencyKey != "idem-1" {
		t.Errorf("row = %+v", r)
	}
	if r.UserID == nil || *r.UserID != "u-1" || r.AppVersion == nil || *r.AppVersion != "1.2.3" ||
		r.Platform == nil || *r.Platform != "ios" {
		t.Errorf("optional fields = %v %v %v", r.UserID, r.AppVersion, r.Platform)
	}
	if string(r.Metadata) != `{"a":1}` {
		t.Errorf("metadata = %s", r.Metadata)
	}
	if r.ReceivedAt != recv.UnixMilli() {
		t.Errorf("ReceivedAt = %d, want %d", r.ReceivedAt, recv.UnixMilli())
	}

	r, err = processor.Normalise(h, []byte(`{"message":"m","userId":"  ","metadata":null}`), testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.UserID != nil || r.AppVersion != nil || r.Platform != nil || r.Metadata != nil {
		t.Errorf("want NULLs, got %+v", r)
	}
}

func TestNormaliseClamps(t *testing.T) {
	cfg := testCfg()
	cfg.MaxMessageChars = 50
	h := hdr("t", "c", "i", time.Now())
	msg := strings.Repeat("é世", 28) // 56 runes
	body := `{"message":"` + msg + `","userId":"` + strings.Repeat("u", 300) + `"}`
	r, err := processor.Normalise(h, []byte(body), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n := utf8.RuneCountInString(r.Message); n != 50 || !utf8.ValidString(r.Message) {
		t.Errorf("message runes = %d valid = %v", n, utf8.ValidString(r.Message))
	}
	if n := utf8.RuneCountInString(*r.UserID); n != 256 {
		t.Errorf("userId runes = %d", n)
	}
}

func TestNormaliseTrim(t *testing.T) {
	r, err := processor.Normalise(hdr("t", "c", "i", time.Now()), []byte(`{"message":"  hi\nthere  "}`), testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Message != "hi\nthere" {
		t.Errorf("message = %q", r.Message)
	}
}

func TestNormaliseErrors(t *testing.T) {
	good := `{"message":"x"}`
	cfg := testCfg()
	cfg.MaxMetadataBytes = 20
	tests := []struct {
		name string
		mod  func(h nats.Header)
		body string
	}{
		{"no tenant", func(h nats.Header) { h.Del(queue.HeaderTenant) }, good},
		{"no channel", func(h nats.Header) { h.Del(queue.HeaderChannel) }, good},
		{"no idem", func(h nats.Header) { h.Del(queue.HeaderIdem) }, good},
		{"no received", func(h nats.Header) { h.Del(queue.HeaderReceived) }, good},
		{"bad received", func(h nats.Header) { h.Set(queue.HeaderReceived, "yesterday") }, good},
		{"not json", nil, `nope`},
		{"trailing data", nil, `{"message":"x"} x`},
		{"null body", nil, `null`},
		{"array", nil, `[{"message":"x"}]`},
		{"message missing", nil, `{}`},
		{"message null", nil, `{"message":null}`},
		{"message blank", nil, `{"message":" \n\t "}`},
		{"message number", nil, `{"message":5}`},
		{"metadata array", nil, `{"message":"x","metadata":[1]}`},
		{"metadata string", nil, `{"message":"x","metadata":"s"}`},
		{"metadata too big", nil, `{"message":"x","metadata":{"k":"` + strings.Repeat("v", 30) + `"}}`},
		{"userId number", nil, `{"message":"x","userId":12}`},
		{"platform object", nil, `{"message":"x","platform":{}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := hdr("t", "c", "i", time.Now())
			if tc.mod != nil {
				tc.mod(h)
			}
			if _, err := processor.Normalise(h, []byte(tc.body), cfg); err == nil {
				t.Error("want error")
			}
		})
	}
}

func TestNormaliseStripsNUL(t *testing.T) {
	body := `{"message":"a\u0000b","userId":"\u0000u","metadata":{"k\u0000":"v\u0000w","n":1.50,"h":"<&>"}}`
	r, err := processor.Normalise(hdr("t", "c", "i", time.Now()), []byte(body), testCfg())
	if err != nil {
		t.Fatal(err)
	}
	if r.Message != "ab" || *r.UserID != "u" {
		t.Errorf("message %q userId %q", r.Message, *r.UserID)
	}
	if string(r.Metadata) != `{"h":"<&>","k":"vw","n":1.50}` {
		t.Errorf("metadata = %s", r.Metadata)
	}
	r, err = processor.Normalise(hdr("t", "c", "i", time.Now()), []byte(`{"message":" \u0000 x"}`), testCfg())
	if err != nil || r.Message != "x" {
		t.Errorf("got %q, %v", r.Message, err)
	}
}
