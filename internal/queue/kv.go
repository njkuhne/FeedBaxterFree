package queue

import (
	"context"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// PutTTL is a compare-and-swap write that carries a per-message TTL.
// nats.go's KeyValue.Update cannot set a TTL, so counters use this for every
// write. lastRev == 0 creates the key; otherwise the write succeeds only if
// lastRev is the key's current revision. Conflicts satisfy IsWrongLastSequence.
func PutTTL(ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue, key string,
	value []byte, lastRev uint64, ttl time.Duration) (uint64, error) {
	if lastRev == 0 {
		return kv.Create(ctx, key, value, jetstream.KeyTTL(ttl))
	}
	ack, err := js.PublishMsg(ctx,
		&nats.Msg{Subject: "$KV." + kv.Bucket() + "." + key, Data: value},
		jetstream.WithExpectLastSequencePerSubject(lastRev),
		jetstream.WithMsgTTL(ttl))
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}
