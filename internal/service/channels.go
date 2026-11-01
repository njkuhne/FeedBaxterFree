package service

import (
	"context"
	"errors"
	"strings"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

func cleanName(name, what string) (string, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > maxNameLen {
		return "", invalid(what + " name must be 1 to 64 characters.")
	}
	return name, nil
}

// ListChannels returns the tenant's channels.
func (s *Service) ListChannels(ctx context.Context, p ext.Principal) ([]store.Channel, error) {
	if err := requireUser(p); err != nil {
		return nil, err
	}
	cs, err := store.ListChannels(ctx, s.DB.SQL, s.d(), p.TenantID)
	if cs == nil {
		cs = []store.Channel{}
	}
	return cs, err
}

// CreateChannel adds a channel (admin only).
func (s *Service) CreateChannel(ctx context.Context, p ext.Principal, name string) (store.Channel, error) {
	if err := requireAdmin(p); err != nil {
		return store.Channel{}, err
	}
	name, err := cleanName(name, "Channel")
	if err != nil {
		return store.Channel{}, err
	}
	c, err := store.CreateChannel(ctx, s.DB.SQL, s.d(), p.TenantID, name, s.nowMs())
	return c, mapErr(err, "", "A channel with that name already exists.")
}

// RenameChannel renames a channel (admin only).
func (s *Service) RenameChannel(ctx context.Context, p ext.Principal, id, name string) error {
	if err := requireAdmin(p); err != nil {
		return err
	}
	name, err := cleanName(name, "Channel")
	if err != nil {
		return err
	}
	return mapErr(store.RenameChannel(ctx, s.DB.SQL, s.d(), p.TenantID, id, name),
		"Channel not found.", "A channel with that name already exists.")
}

// ListKeys returns ingest keys, for one channel or (channelID "") all (admin only).
func (s *Service) ListKeys(ctx context.Context, p ext.Principal, channelID string) ([]store.IngestKey, error) {
	if err := requireAdmin(p); err != nil {
		return nil, err
	}
	if channelID != "" {
		if _, err := store.GetChannel(ctx, s.DB.SQL, s.d(), p.TenantID, channelID); err != nil {
			return nil, mapErr(err, "Channel not found.", "")
		}
	}
	ks, err := store.ListIngestKeys(ctx, s.DB.SQL, s.d(), p.TenantID, channelID)
	if ks == nil {
		ks = []store.IngestKey{}
	}
	return ks, err
}

// CreateKey makes an ingest key for a channel (admin only) and returns the
// plaintext once. A cache sync failure is logged: the reconcile repairs it.
func (s *Service) CreateKey(ctx context.Context, p ext.Principal, channelID string) (store.IngestKey, string, error) {
	if err := requireAdmin(p); err != nil {
		return store.IngestKey{}, "", err
	}
	plain := store.GenerateIngestKey()
	var k store.IngestKey
	err := s.tx(ctx, func(q store.Querier) error {
		if _, err := store.GetChannel(ctx, q, s.d(), p.TenantID, channelID); err != nil {
			return mapErr(err, "Channel not found.", "")
		}
		var err error
		k, err = store.CreateIngestKey(ctx, q, s.d(), p.TenantID, channelID, plain, s.nowMs())
		return err
	})
	if err != nil {
		return store.IngestKey{}, "", err
	}
	if s.Keys != nil {
		if err := s.Keys.SyncKey(ctx, k.ID); err != nil {
			s.log().Warn("service: key cache sync failed; the reconcile will repair it", "key_id", k.ID, "err", err)
		}
	}
	return k, plain, nil
}

// RevokeKey revokes an ingest key (admin only). A cache sync failure is
// returned so the admin retries; revoking again is harmless.
func (s *Service) RevokeKey(ctx context.Context, p ext.Principal, keyID string) (store.IngestKey, error) {
	if err := requireAdmin(p); err != nil {
		return store.IngestKey{}, err
	}
	if _, err := store.GetIngestKey(ctx, s.DB.SQL, s.d(), p.TenantID, keyID); err != nil {
		return store.IngestKey{}, mapErr(err, "Key not found.", "")
	}
	if err := store.RevokeIngestKey(ctx, s.DB.SQL, s.d(), keyID, s.nowMs()); err != nil {
		return store.IngestKey{}, mapErr(err, "Key not found.", "")
	}
	if s.Keys != nil {
		if err := s.Keys.SyncKey(ctx, keyID); err != nil {
			return store.IngestKey{}, err
		}
	}
	k, err := store.GetIngestKey(ctx, s.DB.SQL, s.d(), p.TenantID, keyID)
	return k, mapErr(err, "Key not found.", "")
}

// DeleteChannel removes a channel and its keys (admin only). The keys are
// revoked and synced first so the cache says revoked even if the delete fails.
func (s *Service) DeleteChannel(ctx context.Context, p ext.Principal, id string) error {
	if err := requireAdmin(p); err != nil {
		return err
	}
	var keyIDs []string
	err := s.tx(ctx, func(q store.Querier) error {
		if _, err := store.GetChannel(ctx, q, s.d(), p.TenantID, id); err != nil {
			return mapErr(err, "Channel not found.", "")
		}
		n, err := store.CountChannels(ctx, q, s.d(), p.TenantID)
		if err != nil {
			return err
		}
		if n <= 1 {
			return conflict("Keep at least one channel.")
		}
		keyIDs, err = store.RevokeChannelKeys(ctx, q, s.d(), p.TenantID, id, s.nowMs())
		return err
	})
	if err != nil {
		return err
	}
	if s.Keys != nil {
		var errs []error
		for _, kid := range keyIDs {
			if err := s.Keys.SyncKey(ctx, kid); err != nil {
				errs = append(errs, err)
			}
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
	}
	err = s.tx(ctx, func(q store.Querier) error {
		return mapErr(store.DeleteChannel(ctx, q, s.d(), p.TenantID, id), "Channel not found.", "")
	})
	if err != nil {
		return err
	}
	if s.Keys != nil {
		if err := s.Keys.SyncTenant(ctx, p.TenantID); err != nil {
			s.log().Warn("service: key cache tenant sync failed; the reconcile will repair it", "err", err)
		}
	}
	return nil
}
