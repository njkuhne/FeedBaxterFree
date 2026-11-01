package store

import (
	"regexp"
	"testing"
)

func TestNewID(t *testing.T) {
	prefixes := []Prefix{PrefixFeedback, PrefixChannel, PrefixUser, PrefixTenant,
		PrefixKey, PrefixToken, PrefixTag, PrefixStatusChange}
	for _, p := range prefixes {
		t.Run(string(p), func(t *testing.T) {
			re := regexp.MustCompile("^" + regexp.QuoteMeta(string(p)) + "[a-z2-7]{20}$")
			seen := make(map[string]struct{}, 10000)
			for i := 0; i < 10000; i++ {
				id := NewID(p)
				if len(id) != len(p)+20 {
					t.Fatalf("length %d, want %d", len(id), len(p)+20)
				}
				if !re.MatchString(id) {
					t.Fatalf("bad id %q", id)
				}
				if _, dup := seen[id]; dup {
					t.Fatalf("duplicate id %q", id)
				}
				seen[id] = struct{}{}
			}
		})
	}
}
