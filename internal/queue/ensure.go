package queue

import (
	"context"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// Objects are the handles Ensure declared.
type Objects struct {
	Stream, DLQ    jetstream.Stream
	Consumer       jetstream.Consumer
	Keys, Counters jetstream.KeyValue
}

// Ensure declares the objects of ARCHITECTURE §6 idempotently.
func Ensure(ctx context.Context, js jetstream.JetStream, lim Limits) (*Objects, error) {
	var o Objects
	var err error
	o.Stream, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamFeedback,
		Subjects:   []string{SubjectIngest},
		MaxMsgs:    lim.MaxMsgs,
		MaxBytes:   lim.MaxBytes,
		Discard:    jetstream.DiscardNew,
		Storage:    jetstream.FileStorage,
		Duplicates: DuplicateWindow,
		// WorkQueue removes a message once acked; with LimitsPolicy acked messages
		// would fill the DiscardNew stream and ingest would reject forever.
		Retention: jetstream.WorkQueuePolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: stream %s: %w", StreamFeedback, err)
	}
	o.Consumer, err = o.Stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:   ConsumerProcessor,
		AckPolicy: jetstream.AckExplicitPolicy,
		// Unlimited: a database outage must never strand a message. The processor
		// itself dead-letters rows the database rejects (QUEUE_MAX_DELIVER).
		MaxDeliver:    -1,
		AckWait:       AckWait,
		MaxAckPending: MaxAckPending,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		FilterSubject: SubjectIngest,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: consumer %s: %w", ConsumerProcessor, err)
	}
	o.DLQ, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamDLQ,
		Subjects: []string{SubjectDLQ},
		MaxMsgs:  DLQMaxMsgs,
		Discard:  jetstream.DiscardOld,
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: stream %s: %w", StreamDLQ, err)
	}
	o.Keys, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: BucketKeys, History: 1, Storage: jetstream.FileStorage,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: bucket %s: %w", BucketKeys, err)
	}
	o.Counters, err = js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: BucketCounters, History: 1, Storage: jetstream.FileStorage,
		LimitMarkerTTL: CounterMarkerTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("queue: bucket %s: %w", BucketCounters, err)
	}
	return &o, nil
}
