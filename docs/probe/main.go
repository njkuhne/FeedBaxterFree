package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	_ "modernc.org/sqlite"
)

func rss() string {
	b, _ := os.ReadFile("/proc/self/status")
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "VmRSS") {
			return strings.TrimSpace(strings.TrimPrefix(l, "VmRSS:"))
		}
	}
	return "?"
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	runtime.GC()
	fmt.Println("RSS at start:", rss())

	ns, err := server.NewServer(&server.Options{JetStream: true, StoreDir: "/tmp/js", DontListen: true, NoSigs: true})
	must(err)
	go ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		panic("nats not ready")
	}
	nc, err := nats.Connect("", nats.InProcessServer(ns))
	must(err)
	js, err := jetstream.New(nc)
	must(err)
	ctx := context.Background()
	st, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: "FEEDBACK", Subjects: []string{"feedback.ingest"},
		MaxMsgs: 200000, MaxBytes: 256 << 20, Discard: jetstream.DiscardNew, Storage: jetstream.FileStorage,
	})
	must(err)
	cons, err := st.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{Durable: "processor", AckPolicy: jetstream.AckExplicitPolicy, MaxDeliver: 5})
	must(err)

	db, err := sql.Open("sqlite", "file:/tmp/fb.db?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	must(err)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS feedback(
		id INTEGER PRIMARY KEY, tenant_id TEXT NOT NULL, channel_id TEXT NOT NULL,
		message TEXT NOT NULL, user_id TEXT, metadata TEXT, idempotency_key TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'new', received_at INTEGER NOT NULL,
		UNIQUE(tenant_id, idempotency_key))`)
	must(err)
	time.Sleep(500 * time.Millisecond)
	runtime.GC()
	fmt.Println("RSS idle with embedded JetStream + SQLite open:", rss())

	// Throughput: publish N envelopes (async), then pull in batches and batch-insert.
	const N = 50000
	payload := []byte(`{"tenant":"t1","channel":"c1","key":"k1","received_at":1730000000,"body":{"message":"The export button does nothing on Safari. Steps:\n1. open reports\n2. click export","userId":"usr_8f2a91","idempotencyKey":"IDEMP","appVersion":"2.4.1","platform":"web","metadata":{"plan":"pro","screen":"reports"}}}`)
	t0 := time.Now()
	for i := 0; i < N; i++ {
		msg := strings.Replace(string(payload), "IDEMP", fmt.Sprintf("k-%d", i), 1)
		_, err := js.PublishAsync("feedback.ingest", []byte(msg))
		must(err)
	}
	select {
	case <-js.PublishAsyncComplete():
	case <-time.After(60 * time.Second):
		panic("publish timeout")
	}
	pubDur := time.Since(t0)
	fmt.Printf("publish %d msgs: %v (%.0f msg/s)\n", N, pubDur, float64(N)/pubDur.Seconds())

	t1 := time.Now()
	done := 0
	for done < N {
		batch, err := cons.Fetch(500, jetstream.FetchMaxWait(2*time.Second))
		must(err)
		tx, err := db.Begin()
		must(err)
		stmt, err := tx.Prepare(`INSERT INTO feedback(tenant_id,channel_id,message,user_id,metadata,idempotency_key,received_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(tenant_id,idempotency_key) DO NOTHING`)
		must(err)
		var msgs []jetstream.Msg
		for m := range batch.Messages() {
			d := string(m.Data())
			i := strings.Index(d, "k-")
			j := strings.Index(d[i:], `"`)
			key := d[i : i+j]
			_, err := stmt.Exec("t1", "c1", "The export button does nothing on Safari.", "usr_8f2a91", `{"plan":"pro"}`, key, 1730000000)
			must(err)
			msgs = append(msgs, m)
		}
		must(batch.Error())
		must(stmt.Close())
		must(tx.Commit())
		for _, m := range msgs {
			must(m.Ack())
		}
		done += len(msgs)
	}
	procDur := time.Since(t1)
	fmt.Printf("process %d msgs into SQLite (batch 500, one tx per batch): %v (%.0f msg/s)\n", N, procDur, float64(N)/procDur.Seconds())
	var cnt int
	must(db.QueryRow("SELECT count(*) FROM feedback").Scan(&cnt))
	fmt.Println("rows in sqlite:", cnt)
	runtime.GC()
	fmt.Println("RSS after load:", rss())
	nc.Close()
	ns.Shutdown()
}
