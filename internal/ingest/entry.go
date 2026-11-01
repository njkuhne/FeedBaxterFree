// Package ingest is the hot path of FeedBaxter: key cache, per-key rate limit,
// quota counters, a minimal body check and a synchronous publish. It must not
// import the database (checked by imports_test.go); the DB-reading writer
// lives in the keysync subpackage.
package ingest

import (
	"crypto/sha256"
	"encoding/hex"
)

// Entry is the value of a `keys` KV entry, keyed by HashKey(plaintext).
type Entry struct {
	KeyID             string `json:"key_id"`
	TenantID          string `json:"tenant_id"`
	ChannelID         string `json:"channel_id"`
	DailyLimit        int64  `json:"daily_limit"`   // 0 = unlimited
	MonthlyLimit      int64  `json:"monthly_limit"` // 0 = unlimited
	Revoked           bool   `json:"revoked"`
	BillingMonthStart int    `json:"billing_month_start"` // 1..28; 0 is read as 1
}

// HashKey returns the lowercase hex sha256 of an ingest key. It must equal
// store.HashIngestKey (keysync tests that).
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}
