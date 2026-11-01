// Package service is the transport-free business logic shared by the JSON API
// and the MCP server. Every method that touches tenant data takes an
// ext.Principal, scopes by p.TenantID only and enforces the role itself.
package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/queue"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// Service holds the dependencies of the business logic.
type Service struct {
	DB   *store.DB
	Keys ext.KeyWriter // SyncKey / SyncTenant after key and channel changes; nil skips syncing
	// Queue gives Ops its stream, DLQ and counters handles; nil makes Ops fail.
	Queue         *queue.Objects
	Now           func() time.Time // nil = time.Now
	Log           *slog.Logger     // nil = slog.Default()
	Version, Mode string           // reported by Ops
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) nowMs() int64 { return s.now().UnixMilli() }

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Service) d() store.Dialect { return s.DB.Dialect }

// tx runs fn in a transaction, committing when fn returns nil.
func (s *Service) tx(ctx context.Context, fn func(q store.Querier) error) error {
	tx, err := s.DB.SQL.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("service: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("service: commit: %w", err)
	}
	return nil
}
