package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/ingest"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// Ops is the operations snapshot shown to admins.
type Ops struct {
	Queue struct {
		Depth            uint64
		OldestAgeSeconds float64
		DLQ              uint64
	}
	Counters struct {
		Day, Month       int64
		DayKey, MonthKey string
	}
	DBSizeBytes            int64
	Version, Mode, Dialect string
}

// Ops reports queue depth, counters and database size (admin only).
func (s *Service) Ops(ctx context.Context, p ext.Principal) (Ops, error) {
	var o Ops
	if err := requireAdmin(p); err != nil {
		return o, err
	}
	if s.Queue == nil {
		return o, errors.New("service: ops: queue not configured")
	}
	si, err := s.Queue.Stream.Info(ctx)
	if err != nil {
		return o, fmt.Errorf("service: ops: stream info: %w", err)
	}
	o.Queue.Depth = si.State.Msgs
	if si.State.Msgs > 0 {
		if age := s.now().Sub(si.State.FirstTime).Seconds(); age > 0 {
			o.Queue.OldestAgeSeconds = age
		}
	}
	di, err := s.Queue.DLQ.Info(ctx)
	if err != nil {
		return o, fmt.Errorf("service: ops: dlq info: %w", err)
	}
	o.Queue.DLQ = di.State.Msgs

	tn, err := store.GetTenant(ctx, s.DB.SQL, s.d(), p.TenantID)
	if err != nil {
		return o, mapErr(err, "Organisation not found.", "")
	}
	now := s.now()
	o.Counters.DayKey = ingest.DayKey(p.TenantID, now)
	o.Counters.MonthKey = ingest.MonthKey(p.TenantID, tn.BillingMonthStart, now)
	if o.Counters.Day, err = s.counter(ctx, o.Counters.DayKey); err != nil {
		return o, err
	}
	if o.Counters.Month, err = s.counter(ctx, o.Counters.MonthKey); err != nil {
		return o, err
	}
	if o.DBSizeBytes, err = store.DatabaseSize(ctx, s.DB); err != nil {
		return o, err
	}
	o.Version, o.Mode, o.Dialect = s.Version, s.Mode, string(s.d())
	return o, nil
}

// counter reads a counters-bucket value; a missing key is 0.
func (s *Service) counter(ctx context.Context, key string) (int64, error) {
	e, err := s.Queue.Counters.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("service: ops: read counter: %w", err)
	}
	n, err := strconv.ParseInt(string(e.Value()), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("service: ops: bad counter value for %s", key)
	}
	return n, nil
}
