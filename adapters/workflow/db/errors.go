package db

import (
	"errors"
)

// ErrLeaseHeld is returned by Reclaim when another live owner holds the run.
var ErrLeaseHeld = errors.New("postgres: lease held by another owner")

// LeaseHeldError reports a reclaim against a lease held by another owner.
type LeaseHeldError struct {
	// RunID is the run whose lease is held.
	RunID string
	// Owner is the current lease holder.
	Owner string
}

// Error returns a human-readable lease-held message.
func (e *LeaseHeldError) Error() string {
	return "postgres: run " + e.RunID + " lease held by " + e.Owner
}

// Unwrap returns ErrLeaseHeld.
func (e *LeaseHeldError) Unwrap() error { return ErrLeaseHeld }
