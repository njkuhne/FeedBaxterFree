// Package queue owns the NATS connection (embedded or remote) and declares the
// JetStream objects of ARCHITECTURE §6.
package queue

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/config"
)

const (
	StreamFeedback    = "FEEDBACK"
	SubjectIngest     = "feedback.ingest"
	ConsumerProcessor = "processor"
	StreamDLQ         = "FEEDBACK_DLQ"
	SubjectDLQ        = "feedback.dlq"
	BucketKeys        = "keys"
	BucketCounters    = "counters"

	HeaderTenant   = "FB-Tenant"
	HeaderChannel  = "FB-Channel"
	HeaderKey      = "FB-Key"
	HeaderReceived = "FB-Received"
	HeaderIdem     = "FB-Idem"
	HeaderError    = "FB-Error"
)

// Fixed by ARCHITECTURE §5/§6 (not env vars). Named here so nothing else repeats a literal.
const (
	DuplicateWindow  = 2 * time.Minute
	AckWait          = 30 * time.Second
	MaxAckPending    = 2000
	DLQMaxMsgs       = 10000
	CounterDayTTL    = 40 * 24 * time.Hour
	CounterMonthTTL  = 400 * 24 * time.Hour
	CounterMarkerTTL = time.Hour // LimitMarkerTTL on `counters`; enables per-message TTL
)

const (
	readyTimeout   = 10 * time.Second
	connectTimeout = 5 * time.Second
	drainTimeout   = 5 * time.Second
)

// Limits are the configurable stream caps (QUEUE_MAX_MSGS, QUEUE_MAX_BYTES).
type Limits struct {
	MaxMsgs, MaxBytes int64
}

// LimitsFromConfig takes the caps from cfg.
func LimitsFromConfig(cfg config.Config) Limits {
	return Limits{MaxMsgs: cfg.QueueMaxMsgs, MaxBytes: cfg.QueueMaxBytes}
}

// Conn is a NATS connection plus JetStream context; it owns the embedded server when there is one.
type Conn struct {
	NC *nats.Conn
	JS jetstream.JetStream

	srv    *server.Server // nil when connected to NATS_URL
	closed chan struct{}  // closed by the connection's ClosedCB
	once   sync.Once
}

// Start runs an embedded nats-server and connects to it in-process.
func Start(ctx context.Context, cfg config.Config) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	srv, err := server.NewServer(&server.Options{
		JetStream:  true,
		StoreDir:   filepath.Join(cfg.DataDir, "nats"),
		DontListen: true,
		NoSigs:     true,
		NoLog:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: create embedded server: %w", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(readyTimeout) {
		srv.Shutdown()
		srv.WaitForShutdown()
		return nil, fmt.Errorf("queue: embedded server not ready after %s", readyTimeout)
	}
	c, err := dial(ctx, "", cfg.Mode, nats.InProcessServer(srv))
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		return nil, err
	}
	c.srv = srv
	return c, nil
}

// Connect dials cfg.NATSURL.
func Connect(ctx context.Context, cfg config.Config) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return dial(ctx, cfg.NATSURL, cfg.Mode, nats.MaxReconnects(-1))
}

func dial(_ context.Context, url, mode string, extra ...nats.Option) (*Conn, error) {
	c := &Conn{closed: make(chan struct{})}
	opts := append([]nats.Option{
		nats.Name("feedbaxter-" + mode),
		nats.Timeout(connectTimeout),
		nats.ClosedHandler(func(*nats.Conn) { close(c.closed) }),
	}, extra...)
	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("queue: connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("queue: jetstream: %w", err)
	}
	c.NC, c.JS = nc, js
	return c, nil
}

// Close drains the connection (falling back to Close), then stops the embedded
// server and waits for it. Safe to call twice.
func (c *Conn) Close() {
	c.once.Do(func() {
		if c.NC != nil {
			if err := c.NC.Drain(); err != nil {
				c.NC.Close()
			}
			select {
			case <-c.closed:
			case <-time.After(drainTimeout):
				c.NC.Close()
			}
		}
		if c.srv != nil {
			c.srv.Shutdown()
			c.srv.WaitForShutdown()
		}
	})
}
