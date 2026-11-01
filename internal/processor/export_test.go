package processor

import (
	"time"

	"github.com/njkuhne/feedbaxter/internal/store"
)

// SetDB swaps the database, so tests can take it away and bring it back.
func (p *Processor) SetDB(db *store.DB) { p.db.Store(db) }

// Backoff exposes the redelivery delay for a delivery count.
func (p *Processor) Backoff(delivered uint64) time.Duration { return p.backoff(delivered) }
