package authz

import (
	"errors"
	"fmt"
)

var (
	// ErrUnauthenticated is returned when credentials are missing or invalid.
	ErrUnauthenticated = errors.New("authz: unauthenticated")
	// ErrPermissionDenied is returned when the permission check denies access.
	ErrPermissionDenied = errors.New("authz: permission denied")
)

// UnauthenticatedError reports missing or invalid credentials.
type UnauthenticatedError struct {
	// Reason describes why authentication failed.
	Reason string
}

// Error returns a human-readable unauthenticated message.
func (e UnauthenticatedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnauthenticated, e.Reason)
}

// Unwrap returns ErrUnauthenticated.
func (e UnauthenticatedError) Unwrap() error { return ErrUnauthenticated }

// PermissionDeniedError reports a denied permission check.
type PermissionDeniedError struct {
	// Reason describes why access was denied.
	Reason string
}

// Error returns a human-readable permission-denied message.
func (e PermissionDeniedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrPermissionDenied, e.Reason)
}

// Unwrap returns ErrPermissionDenied.
func (e PermissionDeniedError) Unwrap() error { return ErrPermissionDenied }
