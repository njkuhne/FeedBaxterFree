package store

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// Prefix is the short type tag at the start of an ID.
type Prefix string

// ID prefixes, one per entity.
const (
	PrefixFeedback     Prefix = "fb_"
	PrefixChannel      Prefix = "ch_"
	PrefixUser         Prefix = "us_"
	PrefixTenant       Prefix = "tn_"
	PrefixKey          Prefix = "ky_"
	PrefixToken        Prefix = "tk_"
	PrefixTag          Prefix = "tg_"
	PrefixStatusChange Prefix = "sc_"
)

const idChars = 20

// NewID returns p followed by 20 lowercase base32 characters from crypto/rand.
func NewID(p Prefix) string {
	var b [13]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("store: crypto/rand failed: " + err.Error())
	}
	s := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return string(p) + strings.ToLower(s[:idChars])
}
