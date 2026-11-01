package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/njkuhne/feedbaxter/ext"
	"github.com/njkuhne/feedbaxter/internal/store"
)

const (
	// SessionTTL is how long a session lasts from its last slide.
	SessionTTL        = 30 * 24 * time.Hour
	sessionSlideEvery = time.Hour

	tokenPrefix    = "fbx_pat_"
	tokenRandChars = 32
	tokenDisplay   = 12
	maxNameLen     = 64
)

const msgBadLogin = "Email or password is incorrect."

var (
	dummyOnce sync.Once
	dummyHash string
)

// dummyPHC is verified against when the email is unknown, so the response time
// does not reveal which emails exist.
func dummyPHC() string {
	dummyOnce.Do(func() { dummyHash, _ = HashPassword("not-a-real-password") })
	return dummyHash
}

func normaliseEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func principalOf(u store.User, via string) ext.Principal {
	return ext.Principal{
		UserID: u.ID, TenantID: u.TenantID, Role: ext.Role(u.Role), Via: via,
		MustChangePassword: u.MustChangePassword,
	}
}

// Login checks the email and password of a user of tenantID. Every failure is
// the same KindUnauthenticated error.
func (s *Service) Login(ctx context.Context, tenantID, email, password string) (store.User, error) {
	u, err := store.GetUserByEmail(ctx, s.DB.SQL, s.d(), tenantID, normaliseEmail(email))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			VerifyPassword(dummyPHC(), password)
			return store.User{}, unauthenticated(msgBadLogin)
		}
		return store.User{}, err
	}
	if !VerifyPassword(u.PasswordHash, password) {
		return store.User{}, unauthenticated(msgBadLogin)
	}
	return u, nil
}

// CreateSession starts a session for u and returns the cookie token.
func (s *Service) CreateSession(ctx context.Context, u store.User) (string, time.Time, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, fmt.Errorf("service: session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	now := s.now()
	expires := now.Add(SessionTTL)
	err := store.CreateSession(ctx, s.DB.SQL, s.d(), store.Session{
		ID: sha256Hex(token), TenantID: u.TenantID, UserID: u.ID,
		ExpiresAt: expires.UnixMilli(), CreatedAt: now.UnixMilli(),
	})
	if err != nil {
		return "", time.Time{}, err
	}
	if _, err := store.DeleteExpiredSessions(ctx, s.DB.SQL, s.d(), now.UnixMilli()); err != nil {
		s.log().Warn("service: delete expired sessions", "err", err)
	}
	return token, expires, nil
}

// ResolveSession maps a session cookie token to its principal, sliding the
// expiry at most once per hour.
func (s *Service) ResolveSession(ctx context.Context, token string) (ext.Principal, error) {
	if token == "" {
		return ext.Principal{}, unauthenticated("Sign in to continue.")
	}
	id := sha256Hex(token)
	sess, u, err := store.GetSessionUser(ctx, s.DB.SQL, s.d(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ext.Principal{}, unauthenticated("Sign in to continue.")
		}
		return ext.Principal{}, err
	}
	if sess.TenantID != u.TenantID {
		return ext.Principal{}, unauthenticated("Sign in to continue.")
	}
	now := s.now()
	if sess.ExpiresAt <= now.UnixMilli() {
		if err := store.DeleteSession(ctx, s.DB.SQL, s.d(), id); err != nil {
			s.log().Warn("service: delete expired session", "err", err)
		}
		return ext.Principal{}, unauthenticated("Your session has expired. Sign in again.")
	}
	if sess.ExpiresAt < now.Add(SessionTTL-sessionSlideEvery).UnixMilli() {
		if err := store.SetSessionExpiry(ctx, s.DB.SQL, s.d(), id, now.Add(SessionTTL).UnixMilli()); err != nil {
			s.log().Warn("service: slide session", "err", err)
		}
	}
	return principalOf(u, "ui"), nil
}

// DeleteSession ends the session of token; an absent session is fine.
func (s *Service) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return store.DeleteSession(ctx, s.DB.SQL, s.d(), sha256Hex(token))
}

// ChangePassword sets the caller's own password and ends their other
// sessions, keeping the one that belongs to keepToken. It is allowed while the
// must-change flag is set.
func (s *Service) ChangePassword(ctx context.Context, p ext.Principal, current, next, keepToken string) error {
	if err := requireUser(p); err != nil {
		return err
	}
	u, err := store.GetUser(ctx, s.DB.SQL, s.d(), p.TenantID, p.UserID)
	if err != nil {
		return mapErr(err, "User not found.", "")
	}
	if !VerifyPassword(u.PasswordHash, current) {
		return invalid("Current password is incorrect.")
	}
	if err := checkPasswordRules(next); err != nil {
		return err
	}
	if next == current {
		return invalid("Choose a password different from the current one.")
	}
	hash, err := HashPassword(next)
	if err != nil {
		return err
	}
	keepID := ""
	if keepToken != "" {
		keepID = sha256Hex(keepToken)
	}
	return s.tx(ctx, func(q store.Querier) error {
		if err := store.SetUserPassword(ctx, q, s.d(), p.TenantID, p.UserID, hash, false, s.nowMs()); err != nil {
			return mapErr(err, "User not found.", "")
		}
		_, err := store.DeleteUserSessions(ctx, q, s.d(), p.TenantID, p.UserID, keepID)
		return err
	})
}

// CreateToken makes a personal access token for the caller and returns the
// row and the plaintext, which is shown once.
func (s *Service) CreateToken(ctx context.Context, p ext.Principal, name string) (store.Token, string, error) {
	if err := requireUser(p); err != nil {
		return store.Token{}, "", err
	}
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > maxNameLen {
		return store.Token{}, "", invalid("Give the token a name of 1 to 64 characters.")
	}
	var raw [20]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return store.Token{}, "", fmt.Errorf("service: token: %w", err)
	}
	plain := tokenPrefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:]))
	t, err := store.CreateToken(ctx, s.DB.SQL, s.d(), store.Token{
		TenantID: p.TenantID, UserID: p.UserID, Name: name,
		Prefix: plain[:tokenDisplay], Hash: sha256Hex(plain), CreatedAt: s.nowMs(),
	})
	if err != nil {
		return store.Token{}, "", err
	}
	return t, plain, nil
}

// ListTokens returns the caller's own tokens.
func (s *Service) ListTokens(ctx context.Context, p ext.Principal) ([]store.Token, error) {
	if err := requireUser(p); err != nil {
		return nil, err
	}
	ts, err := store.ListTokens(ctx, s.DB.SQL, s.d(), p.TenantID, p.UserID)
	if ts == nil {
		ts = []store.Token{}
	}
	return ts, err
}

// RevokeToken deletes one of the caller's own tokens.
func (s *Service) RevokeToken(ctx context.Context, p ext.Principal, id string) error {
	if err := requireUser(p); err != nil {
		return err
	}
	return mapErr(store.DeleteToken(ctx, s.DB.SQL, s.d(), p.TenantID, p.UserID, id), "Token not found.", "")
}

func wellFormedToken(t string) bool {
	if len(t) != len(tokenPrefix)+tokenRandChars || !strings.HasPrefix(t, tokenPrefix) {
		return false
	}
	for _, c := range t[len(tokenPrefix):] {
		if !(c >= 'a' && c <= 'z' || c >= '2' && c <= '7') {
			return false
		}
	}
	return true
}

// PrincipalFromToken resolves a personal access token for the MCP server. A
// malformed token never reaches the database.
func (s *Service) PrincipalFromToken(ctx context.Context, token string) (ext.Principal, error) {
	const msg = "Token is invalid or revoked."
	if !wellFormedToken(token) {
		return ext.Principal{}, unauthenticated(msg)
	}
	t, u, err := store.GetTokenUser(ctx, s.DB.SQL, s.d(), sha256Hex(token))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ext.Principal{}, unauthenticated(msg)
		}
		return ext.Principal{}, err
	}
	if t.TenantID != u.TenantID {
		return ext.Principal{}, unauthenticated(msg)
	}
	if err := store.TouchToken(ctx, s.DB.SQL, s.d(), t.ID, s.nowMs()); err != nil {
		s.log().Warn("service: touch token", "err", err)
	}
	p := principalOf(u, "mcp")
	p.MustChangePassword = false
	return p, nil
}
