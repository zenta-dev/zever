package dbconn

import "errors"

// ErrLeaseHeld is returned when a lease is held by a live owner.
var ErrLeaseHeld = errors.New("postgres: lease held by another owner")

// LeaseHeldError reports a claim against a lease held by a live owner.
// Slot identifies a scheduler slot and RunID identifies a workflow run;
// exactly one is set depending on the claiming adapter. Owner is the
// current lease holder.
type LeaseHeldError struct {
	// Slot is the schedule slot whose lease is held.
	Slot string
	// RunID is the workflow run whose lease is held.
	RunID string
	// Owner is the current lease holder.
	Owner string
}

// Error returns a human-readable lease-held message.
func (e *LeaseHeldError) Error() string {
	if e.Slot != "" {
		return "postgres: slot " + e.Slot + " lease held by " + e.Owner
	}
	return "postgres: run " + e.RunID + " lease held by " + e.Owner
}

// Unwrap returns ErrLeaseHeld.
func (e *LeaseHeldError) Unwrap() error { return ErrLeaseHeld }
