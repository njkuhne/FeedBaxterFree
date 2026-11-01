package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	// OWASP's recommended argon2id profile (m=19 MiB, t=2, p=1). The 64 MiB
	// profile hashed just as well but each hash set the GC heap goal to over
	// 128 MB, which the runtime then kept resident for minutes.
	argonTime    = 2
	argonMemory  = 19 * 1024 // KiB
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16

	minPasswordLen = 10
	maxPasswordLen = 256

	tempPasswordLen      = 20
	tempPasswordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
)

// argonSem bounds concurrent argon2 calls: the working set of each must not
// blow the idle memory budget under a login flood.
var argonSem = make(chan struct{}, 2)

// argonKey runs argon2id and then hands the working set straight back to the
// OS. Without that, one hash (19 MiB here, 64 MiB for hashes from older
// installs) doubles the GC heap goal and the process holds the memory for
// minutes; with a live heap of a few MB the forced collection costs well
// under a millisecond.
func argonKey(pw string, salt []byte, t, m uint32, p uint8, n uint32) []byte {
	argonSem <- struct{}{}
	key := argon2.IDKey([]byte(pw), salt, t, m, p, n)
	<-argonSem
	debug.FreeOSMemory()
	return key
}

var b64 = base64.RawStdEncoding

// HashPassword returns the argon2id PHC string of pw.
func HashPassword(pw string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("service: salt: %w", err)
	}
	key := argonKey(pw, salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		19, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the PHC string, comparing in
// constant time. A malformed string never matches.
func VerifyPassword(phc, pw string) bool {
	parts := strings.Split(phc, "$")
	// "", argon2id, v=19, m=..,t=..,p=.., salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var m, t uint32
	var p uint8
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 {
		return false
	}
	// Our own hashes only; refuse parameters that would let a bad row hog memory.
	if m == 0 || m > 256*1024 || t == 0 || t > 10 || p == 0 || p > 16 {
		return false
	}
	salt, err := b64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false
	}
	want, err := b64.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argonKey(pw, salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// GenerateTempPassword returns a 20-character password over an unambiguous
// alphabet, using rejection sampling so there is no modulo bias.
func GenerateTempPassword() string {
	const n = len(tempPasswordAlphabet)
	limit := byte(256 - 256%n) // bytes >= limit are rejected
	out := make([]byte, 0, tempPasswordLen)
	buf := make([]byte, 32)
	for len(out) < tempPasswordLen {
		if _, err := rand.Read(buf); err != nil {
			panic("service: crypto/rand failed: " + err.Error())
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out = append(out, tempPasswordAlphabet[int(b)%n])
			if len(out) == tempPasswordLen {
				break
			}
		}
	}
	return string(out)
}

func checkPasswordRules(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < minPasswordLen {
		return invalid("Use at least 10 characters.")
	}
	if n > maxPasswordLen {
		return invalid("Use at most 256 characters.")
	}
	return nil
}
