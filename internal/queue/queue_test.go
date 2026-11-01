package queue_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/internal/config"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/queue/queuetest"
)

func TestStreamRejectsWhenFullMsgs(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{MaxMsgs: 3})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := c.JS.Publish(ctx, queue.SubjectIngest, []byte{byte('a' + i)}); err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
	_, err := c.JS.Publish(ctx, queue.SubjectIngest, []byte("d"))
	if err == nil || !queue.IsQueueFull(err) {
		t.Fatalf("fourth publish: want queue full, got %v", err)
	}
	info, err := o.Stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 3 || info.State.FirstSeq != 1 || info.State.LastSeq != 3 {
		t.Fatalf("state = %+v, want msgs 1..3", info.State)
	}
}

func TestStreamRejectsWhenFullBytes(t *testing.T) {
	c, _ := queuetest.Embedded(t, queue.Limits{MaxBytes: 1024})
	ctx := context.Background()
	payload := make([]byte, 400)
	var err error
	for i := 0; i < 10 && err == nil; i++ {
		_, err = c.JS.Publish(ctx, queue.SubjectIngest, payload)
	}
	if err == nil || !queue.IsQueueFull(err) {
		t.Fatalf("want queue full, got %v", err)
	}
}

func TestIsQueueFullFalse(t *testing.T) {
	for _, err := range []error{nil, context.DeadlineExceeded, os.ErrNotExist} {
		if queue.IsQueueFull(err) {
			t.Errorf("IsQueueFull(%v) = true", err)
		}
	}
}

func TestDuplicateWindow(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{})
	ctx := context.Background()
	msg := &nats.Msg{Subject: queue.SubjectIngest, Data: []byte("x"), Header: nats.Header{}}
	msg.Header.Set(nats.MsgIdHdr, "id-1")
	a1, err := c.JS.PublishMsg(ctx, msg)
	if err != nil || a1.Duplicate {
		t.Fatalf("first: %+v %v", a1, err)
	}
	a2, err := c.JS.PublishMsg(ctx, msg)
	if err != nil || !a2.Duplicate {
		t.Fatalf("second: %+v %v", a2, err)
	}
	info, _ := o.Stream.Info(ctx)
	if info.State.Msgs != 1 {
		t.Fatalf("msgs = %d, want 1", info.State.Msgs)
	}
}

func TestStartPersistsAndCloses(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{DataDir: dir, Mode: "all"}
	ctx := context.Background()
	lim := queue.Limits{MaxMsgs: 100, MaxBytes: 1 << 20}

	c, err := queue.Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nats")); err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Ensure(ctx, c.JS, lim); err != nil {
		t.Fatal(err)
	}
	if _, err := c.JS.Publish(ctx, queue.SubjectIngest, []byte("persist")); err != nil {
		t.Fatal(err)
	}
	c.Close()
	c.Close()

	c2, err := queue.Start(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	o, err := queue.Ensure(ctx, c2.JS, lim)
	if err != nil {
		t.Fatal(err)
	}
	info, err := o.Stream.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("msgs after restart = %d, want 1", info.State.Msgs)
	}
}

func TestConnectBadURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	c, err := queue.Connect(ctx, config.Config{Mode: "worker", NATSURL: "nats://127.0.0.1:1"})
	if err == nil {
		c.Close()
		t.Fatal("want error")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s", d)
	}
}

// Acked messages must leave the stream, or a DiscardNew stream fills with
// processed messages and rejects new ones forever.
func TestAckedMessagesFreeCapacity(t *testing.T) {
	c, o := queuetest.Embedded(t, queue.Limits{MaxMsgs: 3})
	ctx := context.Background()
	for round := 0; round < 3; round++ {
		for i := 0; i < 3; i++ {
			if _, err := c.JS.Publish(ctx, queue.SubjectIngest, []byte("m")); err != nil {
				t.Fatalf("round %d publish %d: %v", round, i, err)
			}
		}
		batch, err := o.Consumer.Fetch(3, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for m := range batch.Messages() {
			if err := m.DoubleAck(ctx); err != nil {
				t.Fatal(err)
			}
			n++
		}
		if n != 3 {
			t.Fatalf("round %d fetched %d, want 3", round, n)
		}
		info, err := o.Stream.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.State.Msgs != 0 {
			t.Fatalf("round %d: %d messages left after ack, want 0", round, info.State.Msgs)
		}
	}
}
