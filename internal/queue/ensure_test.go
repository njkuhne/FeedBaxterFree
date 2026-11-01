package queue_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxter/internal/queue"
	"github.com/njkuhne/feedbaxter/internal/queue/queuetest"
)

func TestEnsureIdempotent(t *testing.T) {
	c, _ := queuetest.Embedded(t, queue.Limits{MaxMsgs: 50, MaxBytes: 1 << 20})
	ctx := context.Background()
	lim := queue.Limits{MaxMsgs: 50, MaxBytes: 1 << 20}
	o, err := queue.Ensure(ctx, c.JS, lim)
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}

	si, _ := o.Stream.Info(ctx)
	sc := si.Config
	if sc.MaxMsgs != 50 || sc.MaxBytes != 1<<20 || sc.Discard != jetstream.DiscardNew ||
		sc.Duplicates != queue.DuplicateWindow || sc.Storage != jetstream.FileStorage ||
		sc.Retention != jetstream.WorkQueuePolicy {
		t.Errorf("stream config = %+v", sc)
	}
	ci, err := o.Consumer.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cc := ci.Config
	if cc.Durable != queue.ConsumerProcessor || cc.MaxDeliver != -1 || cc.AckWait != queue.AckWait ||
		cc.MaxAckPending != queue.MaxAckPending || cc.AckPolicy != jetstream.AckExplicitPolicy ||
		cc.DeliverPolicy != jetstream.DeliverAllPolicy || cc.FilterSubject != queue.SubjectIngest {
		t.Errorf("consumer config = %+v", cc)
	}
	di, _ := o.DLQ.Info(ctx)
	if di.Config.Discard != jetstream.DiscardOld || di.Config.MaxMsgs != queue.DLQMaxMsgs {
		t.Errorf("dlq config = %+v", di.Config)
	}
	ks, err := o.Keys.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if ks.History() != 1 || ks.LimitMarkerTTL() != 0 {
		t.Errorf("keys: history %d marker %s", ks.History(), ks.LimitMarkerTTL())
	}
	cs, err := o.Counters.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cs.History() != 1 || cs.LimitMarkerTTL() != queue.CounterMarkerTTL {
		t.Errorf("counters: history %d marker %s", cs.History(), cs.LimitMarkerTTL())
	}

	lim.MaxMsgs = 500
	o, err = queue.Ensure(ctx, c.JS, lim)
	if err != nil {
		t.Fatalf("Ensure with larger MaxMsgs: %v", err)
	}
	si, _ = o.Stream.Info(ctx)
	if si.Config.MaxMsgs != 500 {
		t.Errorf("MaxMsgs = %d, want 500", si.Config.MaxMsgs)
	}
}
