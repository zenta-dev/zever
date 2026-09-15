package authz

import (
	"errors"
	"fmt"
)

var (
	ErrUnauthenticated  = errors.New("authz: unauthenticated")
	ErrPermissionDenied = errors.New("authz: permission denied")
)

type UnauthenticatedError struct {
	Reason string
}

func (e UnauthenticatedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrUnauthenticated, e.Reason)
}

func (e UnauthenticatedError) Unwrap() error { return ErrUnauthenticated }

type PermissionDeniedError struct {
	Reason string
}

func (e PermissionDeniedError) Error() string {
	return fmt.Sprintf("%s: %s", ErrPermissionDenied, e.Reason)
}

func (e PermissionDeniedError) Unwrap() error { return ErrPermissionDenied }
