package postgres

import (
	"errors"
)

// ErrLeaseHeld is returned when a slot lease is held by a live owner.
var ErrLeaseHeld = errors.New("postgres: lease held by another owner")

// ErrUnsupportedType is returned when a scanned column has an unexpected Go type.
var ErrUnsupportedType = errors.New("postgres: unsupported type")

// LeaseHeldError reports a slot claim against a lease held by a live owner.
type LeaseHeldError struct {
	// Slot is the schedule slot whose lease is held.
	Slot string
	// Owner is the current lease holder.
	Owner string
}

// Error returns a human-readable lease-held message.
func (e *LeaseHeldError) Error() string {
	return "postgres: slot " + e.Slot + " lease held by " + e.Owner
}

// Unwrap returns ErrLeaseHeld.
func (e *LeaseHeldError) Unwrap() error { return ErrLeaseHeld }
