package processor_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxter/internal/config"
	"github.com/njkuhne/feedbaxter/internal/processor"
	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
	"github.com/njkuhne/feedbaxter/internal/store"
)

const (
	benchFetchMaxWait = 250 * time.Millisecond
	benchPoll         = 10 * time.Millisecond
	benchDefaultBatch = 500
	benchMaxWait      = 10 * time.Minute
)

func benchBody(n int) []byte {
	return fmt.Appendf(nil, `{"message":"The export button on the reports screen does nothing when I tap it.\nI tried twice after updating and then restarted the app, but it still just sits there with no error.","userId":"user-48213","appVersion":"2.14.1","platform":"iOS","metadata":{"locale":"en-GB","screen":"reports","build":"4411"},"idempotencyKey":"bench-%d"}`, n)
}

func benchOpenSQLite(b *testing.B) *store.DB {
	b.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.Database{Driver: "sqlite", Path: filepath.Join(b.TempDir(), "fb.db")})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db, nil); err != nil {
		b.Fatal(err)
	}
	return db
}

// benchOpenPostgres mirrors storetest's isolated schema, which only takes *testing.T.
func benchOpenPostgres(b *testing.B, base string) *store.DB {
	b.Helper()
	ctx := context.Background()
	admin, err := sql.Open("pgx", base)
	if err != nil {
		b.Fatal(err)
	}
	var rnd [6]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		b.Fatal(err)
	}
	schema := "b_" + hex.EncodeToString(rnd[:])
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		_ = admin.Close()
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if _, err := admin.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			b.Errorf("drop schema: %v", err)
		}
		_ = admin.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		b.Fatal("TEST_DATABASE_URL is not a valid URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := store.Open(ctx, config.Database{Driver: "postgres", URL: u.String()})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if _, err := store.Migrate(ctx, db, nil); err != nil {
		b.Fatal(err)
	}
	return db
}

func benchProcess(b *testing.B, db *store.DB, concurrency, batch int) {
	b.Helper()
	ctx := context.Background()
	c, o := queuetest.Embedded(b, queue.Limits{MaxMsgs: int64(b.N) + 1000, MaxBytes: 8 << 30})
	tn, err := store.CreateTenant(ctx, db.SQL, db.Dialect, "bench", 1, 1000)
	if err != nil {
		b.Fatal(err)
	}
	ch, err := store.CreateChannel(ctx, db.SQL, db.Dialect, tn.ID, "main", 1000)
	if err != nil {
		b.Fatal(err)
	}

	// Setup, not the ingest path: async publish, then wait for all acks.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for i := range b.N {
		h := nats.Header{}
		h.Set(queue.HeaderTenant, tn.ID)
		h.Set(queue.HeaderChannel, ch.ID)
		h.Set(queue.HeaderKey, "ky_bench")
		h.Set(queue.HeaderReceived, now)
		h.Set(queue.HeaderIdem, fmt.Sprintf("bench-%d", i))
		h.Set(jetstream.MsgIDHeader, fmt.Sprintf("%s:bench-%d", tn.ID, i))
		if _, err := c.JS.PublishMsgAsync(&nats.Msg{Subject: queue.SubjectIngest, Header: h, Data: benchBody(i)}); err != nil {
			b.Fatalf("publish %d: %v", i, err)
		}
	}
	select {
	case <-c.JS.PublishAsyncComplete():
	case <-time.After(benchMaxWait):
		b.Fatal("publish did not complete")
	}

	p := processor.New(db, c.JS, o.Consumer, processor.Config{
		Concurrency:      concurrency,
		BatchSize:        batch,
		FetchMaxWait:     benchFetchMaxWait,
		MaxDeliver:       5,
		MaxMessageChars:  5000,
		MaxMetadataBytes: 4096,
		NakDelay:         processor.DefaultNakDelay,
	}, nil)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)

	b.ResetTimer()
	go func() { done <- p.Run(runCtx) }()
	deadline := time.Now().Add(benchMaxWait)
	tick := time.NewTicker(benchPoll)
	defer tick.Stop()
	for {
		var n int
		if err := db.SQL.QueryRow("SELECT COUNT(*) FROM feedback").Scan(&n); err != nil {
			b.Fatal(err)
		}
		if n == b.N {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			b.Fatalf("stored %d of %d rows before timeout", n, b.N)
		}
		<-tick.C
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "msg/s")
	cancel()
	if err := <-done; err != nil {
		b.Errorf("Run: %v", err)
	}
	if si, err := o.DLQ.Info(ctx); err != nil || si.State.Msgs != 0 {
		b.Errorf("dlq msgs %v, err %v", si, err)
	}
}

func BenchmarkProcessor(b *testing.B) {
	b.Run("batch=100", func(b *testing.B) { benchProcess(b, benchOpenSQLite(b), 1, 100) })
	b.Run("batch=500", func(b *testing.B) { benchProcess(b, benchOpenSQLite(b), 1, benchDefaultBatch) })
	b.Run("postgres", func(b *testing.B) {
		base := os.Getenv("TEST_DATABASE_URL")
		if base == "" {
			b.Skip("TEST_DATABASE_URL not set; skipping postgres")
		}
		benchProcess(b, benchOpenPostgres(b, base), 4, benchDefaultBatch)
	})
}
