package service

import (
	"context"
	"strings"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

const maxEmailLen = 254

func validRole(r string) bool { return r == string(ext.RoleAdmin) || r == string(ext.RoleReviewer) }

// Me returns the caller's own user row.
func (s *Service) Me(ctx context.Context, p ext.Principal) (store.User, error) {
	if err := requireUser(p); err != nil {
		return store.User{}, err
	}
	u, err := store.GetUser(ctx, s.DB.SQL, s.d(), p.TenantID, p.UserID)
	return u, mapErr(err, "User not found.", "")
}

// ListUsers returns the tenant's users (admin only).
func (s *Service) ListUsers(ctx context.Context, p ext.Principal) ([]store.User, error) {
	if err := requireAdmin(p); err != nil {
		return nil, err
	}
	us, err := store.ListUsers(ctx, s.DB.SQL, s.d(), p.TenantID)
	if us == nil {
		us = []store.User{}
	}
	return us, err
}

// CreateUser adds a user with a temporary password that must be changed at
// first sign-in (admin only). The plaintext password is returned once.
func (s *Service) CreateUser(ctx context.Context, p ext.Principal, email, role string) (store.User, string, error) {
	if err := requireAdmin(p); err != nil {
		return store.User{}, "", err
	}
	email = normaliseEmail(email)
	if len(email) == 0 || len(email) > maxEmailLen || strings.Count(email, "@") != 1 ||
		strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") || strings.ContainsAny(email, " \t\r\n") {
		return store.User{}, "", invalid("Enter a valid email address.")
	}
	if !validRole(role) {
		return store.User{}, "", invalid("Role must be admin or reviewer.")
	}
	temp := GenerateTempPassword()
	hash, err := HashPassword(temp)
	if err != nil {
		return store.User{}, "", err
	}
	now := s.nowMs()
	u, err := store.CreateUser(ctx, s.DB.SQL, s.d(), store.User{
		TenantID: p.TenantID, Email: email, PasswordHash: hash, Role: role,
		MustChangePassword: true, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return store.User{}, "", mapErr(err, "", "A user with that email already exists.")
	}
	return u, temp, nil
}

// ResetPassword gives another user a new temporary password, forces a change
// and ends their sessions; their tokens stay (admin only).
func (s *Service) ResetPassword(ctx context.Context, p ext.Principal, userID string) (string, error) {
	if err := requireAdmin(p); err != nil {
		return "", err
	}
	if userID == p.UserID {
		return "", invalid("Use change password for your own account.")
	}
	temp := GenerateTempPassword()
	hash, err := HashPassword(temp)
	if err != nil {
		return "", err
	}
	err = s.tx(ctx, func(q store.Querier) error {
		if err := store.SetUserPassword(ctx, q, s.d(), p.TenantID, userID, hash, true, s.nowMs()); err != nil {
			return mapErr(err, "User not found.", "")
		}
		_, err := store.DeleteUserSessions(ctx, q, s.d(), p.TenantID, userID, "")
		return err
	})
	if err != nil {
		return "", err
	}
	return temp, nil
}

const msgLastAdmin = "An organisation needs at least one admin."

// SetRole changes a user's role (admin only). Demoting the last admin is refused.
func (s *Service) SetRole(ctx context.Context, p ext.Principal, userID, role string) error {
	if err := requireAdmin(p); err != nil {
		return err
	}
	if !validRole(role) {
		return invalid("Role must be admin or reviewer.")
	}
	return s.tx(ctx, func(q store.Querier) error {
		if err := store.LockAdmins(ctx, q, s.d(), p.TenantID); err != nil {
			return err
		}
		u, err := store.GetUser(ctx, q, s.d(), p.TenantID, userID)
		if err != nil {
			return mapErr(err, "User not found.", "")
		}
		if u.Role == string(ext.RoleAdmin) && role != string(ext.RoleAdmin) {
			n, err := store.CountAdmins(ctx, q, s.d(), p.TenantID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return conflict(msgLastAdmin)
			}
		}
		return mapErr(store.SetUserRole(ctx, q, s.d(), p.TenantID, userID, role, s.nowMs()), "User not found.", "")
	})
}

// DeleteUser removes a user with their sessions and tokens (admin only).
func (s *Service) DeleteUser(ctx context.Context, p ext.Principal, userID string) error {
	if err := requireAdmin(p); err != nil {
		return err
	}
	if userID == p.UserID {
		return conflict("You cannot remove yourself.")
	}
	return s.tx(ctx, func(q store.Querier) error {
		if err := store.LockAdmins(ctx, q, s.d(), p.TenantID); err != nil {
			return err
		}
		u, err := store.GetUser(ctx, q, s.d(), p.TenantID, userID)
		if err != nil {
			return mapErr(err, "User not found.", "")
		}
		if u.Role == string(ext.RoleAdmin) {
			n, err := store.CountAdmins(ctx, q, s.d(), p.TenantID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return conflict(msgLastAdmin)
			}
		}
		return mapErr(store.DeleteUser(ctx, q, s.d(), p.TenantID, userID), "User not found.", "")
	})
}
