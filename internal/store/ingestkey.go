package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// IngestKeyPrefix starts every ingest key.
const IngestKeyPrefix = "fbx_"

const displayPrefixLen = len(IngestKeyPrefix) + 8

// GenerateIngestKey returns "fbx_" followed by 32 lowercase base32 characters
// from crypto/rand.
func GenerateIngestKey() string {
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("store: crypto/rand failed: " + err.Error())
	}
	return IngestKeyPrefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}

// HashIngestKey returns the lowercase hex sha256 of key.
func HashIngestKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// DisplayPrefix returns the part of key that is safe to show.
func DisplayPrefix(key string) string {
	if len(key) < displayPrefixLen {
		return key
	}
	return key[:displayPrefixLen]
}

// IngestKey is one row of ingest_keys. The plaintext key is never stored.
type IngestKey struct {
	ID, TenantID, ChannelID, Prefix, Hash string
	CreatedAt                             int64
	RevokedAt                             *int64
	LastUsedAt                            *int64 // Unix ms of the last stored feedback; nil until first use
}

// CreateIngestKey stores the hash and display prefix of plaintext.
func CreateIngestKey(ctx context.Context, q Querier, d Dialect, tenantID, channelID, plaintext string, now int64) (IngestKey, error) {
	k := IngestKey{
		ID: NewID(PrefixKey), TenantID: tenantID, ChannelID: channelID,
		Prefix: DisplayPrefix(plaintext), Hash: HashIngestKey(plaintext), CreatedAt: now,
	}
	_, err := q.ExecContext(ctx, d.Rebind(
		"INSERT INTO ingest_keys (id, tenant_id, channel_id, prefix, hash, created_at) VALUES (?, ?, ?, ?, ?, ?)"),
		k.ID, k.TenantID, k.ChannelID, k.Prefix, k.Hash, k.CreatedAt)
	if err != nil {
		return IngestKey{}, fmt.Errorf("store: create ingest key: %w", err)
	}
	return k, nil
}

// RevokeIngestKey sets revoked_at to now unless already revoked. It returns
// ErrNotFound when no key has that id.
func RevokeIngestKey(ctx context.Context, q Querier, d Dialect, id string, now int64) error {
	res, err := q.ExecContext(ctx, d.Rebind(
		"UPDATE ingest_keys SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL"), now, id)
	if err != nil {
		return fmt.Errorf("store: revoke ingest key: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: revoke ingest key: %w", err)
	}
	if n > 0 {
		return nil
	}
	var one int
	err = q.QueryRowContext(ctx, d.Rebind("SELECT 1 FROM ingest_keys WHERE id = ?"), id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: revoke ingest key: %w", ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("store: revoke ingest key: %w", err)
	}
	return nil // already revoked
}

// CacheKey is one ingest key as the ingest key cache needs it.
type CacheKey struct {
	KeyID, Hash, TenantID, ChannelID string
	Revoked                          bool
	BillingMonthStart                int
}

const cacheKeySelect = "SELECT k.id, k.hash, k.tenant_id, k.channel_id, k.revoked_at IS NOT NULL, t.billing_month_start " +
	"FROM ingest_keys k JOIN tenants t ON t.id = k.tenant_id"

func scanCacheKey(s interface{ Scan(...any) error }) (CacheKey, error) {
	var c CacheKey
	err := s.Scan(&c.KeyID, &c.Hash, &c.TenantID, &c.ChannelID, &c.Revoked, &c.BillingMonthStart)
	return c, err
}

func listCacheKeys(ctx context.Context, q Querier, d Dialect, where string, args ...any) ([]CacheKey, error) {
	rows, err := q.QueryContext(ctx, d.Rebind(cacheKeySelect+where+" ORDER BY k.id"), args...)
	if err != nil {
		return nil, fmt.Errorf("store: list ingest keys: %w", err)
	}
	defer rows.Close()
	var out []CacheKey
	for rows.Next() {
		c, err := scanCacheKey(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list ingest keys: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list ingest keys: %w", err)
	}
	return out, nil
}

// ListIngestKeysForCache returns every key, revoked included, ordered by id.
func ListIngestKeysForCache(ctx context.Context, q Querier, d Dialect) ([]CacheKey, error) {
	return listCacheKeys(ctx, q, d, "")
}

// ListIngestKeysForCacheByTenant is ListIngestKeysForCache for one tenant.
func ListIngestKeysForCacheByTenant(ctx context.Context, q Querier, d Dialect, tenantID string) ([]CacheKey, error) {
	return listCacheKeys(ctx, q, d, " WHERE k.tenant_id = ?", tenantID)
}

// GetIngestKeyForCache returns ErrNotFound when keyID does not exist.
func GetIngestKeyForCache(ctx context.Context, q Querier, d Dialect, keyID string) (CacheKey, error) {
	c, err := scanCacheKey(q.QueryRowContext(ctx, d.Rebind(cacheKeySelect+" WHERE k.id = ?"), keyID))
	if errors.Is(err, sql.ErrNoRows) {
		return CacheKey{}, fmt.Errorf("store: get ingest key: %w", ErrNotFound)
	}
	if err != nil {
		return CacheKey{}, fmt.Errorf("store: get ingest key: %w", err)
	}
	return c, nil
}

const ingestKeyCols = "id, tenant_id, channel_id, prefix, hash, created_at, revoked_at, last_used_at"

// ListIngestKeys returns the tenant's keys, newest first; channelID "" lists
// every channel's keys.
func ListIngestKeys(ctx context.Context, q Querier, d Dialect, tenantID, channelID string) ([]IngestKey, error) {
	query := "SELECT " + ingestKeyCols + " FROM ingest_keys WHERE tenant_id = ?"
	args := []any{tenantID}
	if channelID != "" {
		query += " AND channel_id = ?"
		args = append(args, channelID)
	}
	rows, err := q.QueryContext(ctx, d.Rebind(query+" ORDER BY created_at DESC, id"), args...)
	if err != nil {
		return nil, fmt.Errorf("store: list ingest keys: %w", err)
	}
	defer rows.Close()
	var out []IngestKey
	for rows.Next() {
		var k IngestKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.ChannelID, &k.Prefix, &k.Hash, &k.CreatedAt, &k.RevokedAt, &k.LastUsedAt); err != nil {
			return nil, fmt.Errorf("store: list ingest keys: %w", err)
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list ingest keys: %w", err)
	}
	return out, nil
}

// GetIngestKey returns ErrNotFound when the key is not in the tenant.
func GetIngestKey(ctx context.Context, q Querier, d Dialect, tenantID, id string) (IngestKey, error) {
	var k IngestKey
	err := q.QueryRowContext(ctx, d.Rebind(
		"SELECT "+ingestKeyCols+" FROM ingest_keys WHERE tenant_id = ? AND id = ?"), tenantID, id).
		Scan(&k.ID, &k.TenantID, &k.ChannelID, &k.Prefix, &k.Hash, &k.CreatedAt, &k.RevokedAt, &k.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return IngestKey{}, fmt.Errorf("store: get ingest key: %w", ErrNotFound)
	}
	if err != nil {
		return IngestKey{}, fmt.Errorf("store: get ingest key: %w", err)
	}
	return k, nil
}

// RevokeChannelKeys sets revoked_at on the channel's unrevoked keys and
// returns the IDs of all of the channel's keys.
func RevokeChannelKeys(ctx context.Context, q Querier, d Dialect, tenantID, channelID string, now int64) ([]string, error) {
	_, err := q.ExecContext(ctx, d.Rebind(
		"UPDATE ingest_keys SET revoked_at = ? WHERE tenant_id = ? AND channel_id = ? AND revoked_at IS NULL"),
		now, tenantID, channelID)
	if err != nil {
		return nil, fmt.Errorf("store: revoke channel keys: %w", err)
	}
	rows, err := q.QueryContext(ctx, d.Rebind(
		"SELECT id FROM ingest_keys WHERE tenant_id = ? AND channel_id = ? ORDER BY id"), tenantID, channelID)
	if err != nil {
		return nil, fmt.Errorf("store: revoke channel keys: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: revoke channel keys: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: revoke channel keys: %w", err)
	}
	return ids, nil
}

// KeyTouch records that a key was used at At (Unix ms).
type KeyTouch struct {
	TenantID, KeyID string
	At              int64
}

// TouchIngestKeys sets last_used_at = at for each key in the tenant whose
// last_used_at is NULL or older than at. Unknown or deleted keys are ignored.
func TouchIngestKeys(ctx context.Context, q Querier, d Dialect, touches []KeyTouch) error {
	for _, t := range touches {
		_, err := q.ExecContext(ctx, d.Rebind(
			"UPDATE ingest_keys SET last_used_at = ? WHERE tenant_id = ? AND id = ? AND (last_used_at IS NULL OR last_used_at < ?)"),
			t.At, t.TenantID, t.KeyID, t.At)
		if err != nil {
			return fmt.Errorf("store: touch ingest keys: %w", err)
		}
	}
	return nil
}
