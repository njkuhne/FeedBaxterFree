package store

import "errors"

// ErrNotFound is returned when a row looked up by key does not exist.
var ErrNotFound = errors.New("store: not found")
