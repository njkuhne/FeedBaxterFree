package service

import (
	"errors"

	"github.com/njkuhne/feedbaxterfree/ext"
	"github.com/njkuhne/feedbaxterfree/internal/store"
)

// Kind classifies a service error for the transport layer.
type Kind int

const (
	// KindInvalid is bad input (HTTP 400).
	KindInvalid Kind = iota + 1
	// KindUnauthenticated is a missing or bad credential (HTTP 401).
	KindUnauthenticated
	// KindForbidden is a role that may not do this (HTTP 403).
	KindForbidden
	// KindNotFound is an unknown or foreign object (HTTP 404).
	KindNotFound
	// KindConflict is a state clash (HTTP 409).
	KindConflict
)

// Error carries a Kind and a plain sentence that is safe to show to the caller.
// Anything that is not an *Error is internal: the API logs it and answers 500
// without detail.
type Error struct {
	Kind Kind
	Msg  string
}

// Error returns the caller-safe message.
func (e *Error) Error() string { return e.Msg }

// KindOf returns the Kind of err, or 0 when err is not an *Error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}

func invalid(msg string) error         { return &Error{Kind: KindInvalid, Msg: msg} }
func unauthenticated(msg string) error { return &Error{Kind: KindUnauthenticated, Msg: msg} }
func conflict(msg string) error        { return &Error{Kind: KindConflict, Msg: msg} }
func notFound(msg string) error        { return &Error{Kind: KindNotFound, Msg: msg} }

// mapErr turns store.ErrNotFound and store.ErrConflict into *Error with the
// given sentences (an empty sentence leaves that case internal) and passes
// every other error through unchanged.
func mapErr(err error, notFoundMsg, conflictMsg string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound) && notFoundMsg != "":
		return notFound(notFoundMsg)
	case errors.Is(err, store.ErrConflict) && conflictMsg != "":
		return conflict(conflictMsg)
	}
	return err
}

// requireUser is the entry check of every method that touches tenant data.
func requireUser(p ext.Principal) error {
	if p.UserID == "" || p.TenantID == "" || (p.Role != ext.RoleAdmin && p.Role != ext.RoleReviewer) {
		return unauthenticated("Sign in to continue.")
	}
	return nil
}

// requireAdmin is the only role check.
func requireAdmin(p ext.Principal) error {
	if err := requireUser(p); err != nil {
		return err
	}
	if p.Role != ext.RoleAdmin {
		return &Error{Kind: KindForbidden, Msg: "Only admins can do that."}
	}
	return nil
}
