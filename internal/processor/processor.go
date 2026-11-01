// Package processor pulls batches from the processor consumer, normalises
// them and inserts them in one transaction (ARCHITECTURE §7).
package processor

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/store"
)

// DefaultNakDelay is the base redelivery delay after a failed insert (§7).
const DefaultNakDelay = time.Second

const (
	// drainTimeout bounds finishing an already fetched batch after shutdown.
	drainTimeout      = 30 * time.Second
	fetchErrorBackoff = time.Second
	// maxNakBackoff caps the redelivery delay and the worker pause after a failure.
	maxNakBackoff = 30 * time.Second
	// pingTimeout bounds the reachability probe that classifies an insert error.
	pingTimeout = 2 * time.Second
)

// Config holds the processor's settings.
type Config struct {
	Concurrency      int           // WORKER_CONCURRENCY
	BatchSize        int           // WORKER_BATCH_SIZE
	FetchMaxWait     time.Duration // WORKER_FETCH_MAX_WAIT
	MaxDeliver       int           // QUEUE_MAX_DELIVER
	MaxMessageChars  int
	MaxMetadataBytes int64
	RetentionWindow  time.Duration
	// RetentionCheckInterval is how often retention runs; <= 0 disables it.
	RetentionCheckInterval time.Duration
	NakDelay               time.Duration // base for NakWithDelay (times NumDelivered)
}

// ConfigFrom takes the processor settings from cfg.
func ConfigFrom(cfg config.Config) Config {
	return Config{
		Concurrency:            cfg.WorkerConcurrency,
		BatchSize:              cfg.WorkerBatchSize,
		FetchMaxWait:           cfg.WorkerFetchMaxWait,
		MaxDeliver:             cfg.QueueMaxDeliver,
		MaxMessageChars:        cfg.MaxMessageChars,
		MaxMetadataBytes:       cfg.MaxMetadataBytes,
		RetentionWindow:        cfg.RetentionWindow,
		RetentionCheckInterval: cfg.RetentionCheckInterval,
		NakDelay:               DefaultNakDelay,
	}
}

// Processor consumes feedback messages and stores them.
type Processor struct {
	db   atomic.Pointer[store.DB] // swappable only by tests (SetDB)
	js   jetstream.JetStream
	cons jetstream.Consumer
	cfg  Config
	log  *slog.Logger
}

// New returns a Processor; call Run to start it.
func New(db *store.DB, js jetstream.JetStream, cons jetstream.Consumer, cfg Config, log *slog.Logger) *Processor {
	if log == nil {
		log = slog.Default()
	}
	p := &Processor{js: js, cons: cons, cfg: cfg, log: log}
	p.db.Store(db)
	return p
}

// Run starts the workers and the retention loop and blocks until ctx is done
// and all of them have returned. A fetched batch is finished on a context
// detached from ctx; nothing new is fetched once ctx is done.
func (p *Processor) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for range max(p.cfg.Concurrency, 1) {
		wg.Go(func() { p.worker(ctx) })
	}
	wg.Go(func() { p.retentionLoop(ctx) })
	wg.Wait()
	return nil
}

func (p *Processor) worker(ctx context.Context) {
	for ctx.Err() == nil {
		batch, err := p.cons.Fetch(p.cfg.BatchSize, jetstream.FetchMaxWait(p.cfg.FetchMaxWait))
		if err != nil {
			p.log.Warn("fetch failed", "err", err)
			pause(ctx, fetchErrorBackoff)
			continue
		}
		var msgs []jetstream.Msg
		for m := range batch.Messages() {
			msgs = append(msgs, m)
		}
		// An empty or timed-out fetch leaves Error() nil in nats.go v1.55
		// (ErrTimeout and ErrNoMessages are swallowed in fetch()).
		fetchErr := batch.Error()
		var wait time.Duration
		if len(msgs) > 0 {
			fin, cancel := context.WithTimeout(context.WithoutCancel(ctx), drainTimeout)
			wait = p.handle(fin, msgs)
			cancel()
		}
		if fetchErr != nil && !errors.Is(fetchErr, nats.ErrTimeout) {
			p.log.Warn("fetch failed", "err", fetchErr)
			wait = max(wait, fetchErrorBackoff)
		}
		pause(ctx, wait)
	}
}

// pause waits d or until ctx is done.
func pause(ctx context.Context, d time.Duration) {
	if d <= 0 || ctx.Err() != nil {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// backoff is the redelivery delay for a message delivered n times.
func (p *Processor) backoff(delivered uint64) time.Duration {
	d := p.cfg.NakDelay * time.Duration(delivered)
	if d < 0 || d > maxNakBackoff {
		return maxNakBackoff
	}
	return d
}

type item struct {
	msg jetstream.Msg
	row store.FeedbackRow
}

// handle settles every message and returns how long the worker should pause
// before fetching again (non-zero after a failure that naked messages).
func (p *Processor) handle(ctx context.Context, msgs []jetstream.Msg) (wait time.Duration) {
	good := make([]item, 0, len(msgs))
	bad := 0
	for _, m := range msgs {
		row, err := Normalise(m.Headers(), m.Data(), p.cfg)
		if err != nil {
			bad++
			if p.deadLetter(ctx, m, err.Error()) {
				logIf(p.log, "term", m.Term())
			} else {
				wait = max(wait, p.nak(m))
			}
			continue
		}
		good = append(good, item{m, row})
	}
	if len(good) == 0 {
		p.log.Debug("batch handled", "fetched", len(msgs), "stored", 0, "dead", bad)
		return wait
	}

	db := p.db.Load()
	inserted, err := p.insert(ctx, db, good)
	if err == nil {
		for _, it := range good {
			logIf(p.log, "ack", it.msg.Ack())
		}
		p.log.Debug("batch handled", "fetched", len(msgs), "inserted", inserted,
			"duplicates", len(good)-inserted, "dead", bad)
		return wait
	}

	p.log.Warn("batch insert failed", "rows", len(good), "err", err)
	down := p.transient(ctx, db, err)
	// Otherwise isolate a poison row: retry each row in its own transaction (a
	// failed statement aborts a Postgres transaction, so they cannot share one).
	// After the first transient error the rest are not tried.
	stored := 0
	for _, it := range good {
		if !down {
			if _, rerr := p.insert(ctx, db, []item{it}); rerr == nil {
				stored++
				logIf(p.log, "ack", it.msg.Ack())
				continue
			} else {
				err = rerr
				down = p.transient(ctx, db, rerr)
			}
		}
		if down {
			wait = max(wait, p.nak(it.msg))
		} else {
			wait = max(wait, p.settleRejected(ctx, it.msg, err))
		}
	}
	p.log.Debug("batch handled per row", "fetched", len(msgs), "stored", stored, "dead", bad, "unreachable", down)
	return wait
}

// nak asks for redelivery after the backoff for m's delivery count.
func (p *Processor) nak(m jetstream.Msg) time.Duration {
	d := p.backoff(p.delivered(m))
	logIf(p.log, "nak", m.NakWithDelay(d))
	return d
}

// settleRejected handles a row the database rejected: retry until MaxDeliver
// attempts, then dead-letter it.
func (p *Processor) settleRejected(ctx context.Context, m jetstream.Msg, err error) time.Duration {
	if p.delivered(m) >= uint64(p.cfg.MaxDeliver) {
		reason := err.Error()
		if !strings.HasPrefix(reason, "store: ") {
			reason = "store: " + reason
		}
		if p.deadLetter(ctx, m, reason) {
			logIf(p.log, "ack", m.Ack())
			return 0
		}
		return p.nak(m) // DLQ publish failed: infrastructure trouble, pause
	}
	// A rejected row is the row's problem, not the infrastructure's: the
	// worker must not stall healthy traffic behind it.
	p.nak(m)
	return 0
}

var (
	errBegin  = errors.New("begin transaction")
	errCommit = errors.New("commit transaction")
)

// transient reports whether err means the database is unreachable rather
// than that it rejected the rows. Such failures must never reach the DLQ.
func (p *Processor) transient(ctx context.Context, db *store.DB, err error) bool {
	var ne net.Error
	switch {
	case errors.Is(err, errBegin), errors.Is(err, errCommit),
		errors.Is(err, sql.ErrConnDone), errors.Is(err, driver.ErrBadConn),
		errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled),
		errors.As(err, &ne):
		return true
	}
	// Driver-specific connection errors: if the database does not answer a
	// ping either, the error says nothing about the row.
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pingTimeout)
	defer cancel()
	return db.SQL.PingContext(pctx) != nil
}

func (p *Processor) insert(ctx context.Context, db *store.DB, items []item) (int, error) {
	rows := make([]store.FeedbackRow, len(items))
	for i, it := range items {
		rows[i] = it.row
	}
	tx, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errBegin, err)
	}
	defer func() { _ = tx.Rollback() }()
	n, err := store.InsertFeedbackBatch(ctx, tx, db.Dialect, rows)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%w: %w", errCommit, err)
	}
	return n, nil
}

// delivered returns the delivery count; if the metadata is unreadable it
// reports 1 so the message is retried rather than dead-lettered.
func (p *Processor) delivered(m jetstream.Msg) uint64 {
	md, err := m.Metadata()
	if err != nil || md.NumDelivered < 1 {
		return 1
	}
	return md.NumDelivered
}

// deadLetter publishes a copy of m with every original header plus FB-Error
// to the DLQ and reports whether the copy is stored. On false the caller must
// neither Ack nor Term m.
func (p *Processor) deadLetter(ctx context.Context, m jetstream.Msg, reason string) bool {
	hdr := nats.Header{}
	for k, v := range m.Headers() {
		hdr[k] = append([]string(nil), v...)
	}
	hdr.Set(queue.HeaderError, sanitize(reason))
	_, err := p.js.PublishMsg(ctx, &nats.Msg{Subject: queue.SubjectDLQ, Header: hdr, Data: m.Data()})
	if err != nil {
		p.log.Warn("dead-letter publish failed; message will be redelivered", "err", err)
		return false
	}
	p.log.Warn("message dead-lettered", "reason", reason,
		"tenant", m.Headers().Get(queue.HeaderTenant), "idem", m.Headers().Get(queue.HeaderIdem))
	return true
}

// sanitize makes reason safe as a header value (no line breaks).
func sanitize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func logIf(log *slog.Logger, what string, err error) {
	if err != nil {
		log.Warn("message settle failed", "op", what, "err", err)
	}
}
