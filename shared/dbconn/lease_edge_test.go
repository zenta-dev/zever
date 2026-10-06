package dbconn

import (
	"errors"
	"testing"
)

// TestLeaseHeldError_slotMessage verifies the slot form of the message.
func TestLeaseHeldError_slotMessage(t *testing.T) {
	t.Parallel()

	e := &LeaseHeldError{Slot: "slot-1", Owner: "owner-a"}
	want := "postgres: slot slot-1 lease held by owner-a"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestLeaseHeldError_runMessage verifies the run form of the message.
func TestLeaseHeldError_runMessage(t *testing.T) {
	t.Parallel()

	e := &LeaseHeldError{RunID: "run-9", Owner: "owner-b"}
	want := "postgres: run run-9 lease held by owner-b"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestLeaseHeldError_slotTakesPrecedence verifies Slot wins when both
// identifiers are set.
func TestLeaseHeldError_slotTakesPrecedence(t *testing.T) {
	t.Parallel()

	e := &LeaseHeldError{Slot: "slot-1", RunID: "run-9", Owner: "owner-a"}
	want := "postgres: slot slot-1 lease held by owner-a"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestLeaseHeldError_emptyIdentifiers verifies the message degrades when
// neither Slot nor RunID is set.
func TestLeaseHeldError_emptyIdentifiers(t *testing.T) {
	t.Parallel()

	e := &LeaseHeldError{Owner: "owner-a"}
	want := "postgres: run  lease held by owner-a"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestLeaseHeldError_unwrapMatchesSentinel verifies errors.Is resolves to
// ErrLeaseHeld through Unwrap.
func TestLeaseHeldError_unwrapMatchesSentinel(t *testing.T) {
	t.Parallel()

	e := &LeaseHeldError{Slot: "slot-1", Owner: "owner-a"}
	if !errors.Is(e, ErrLeaseHeld) {
		t.Errorf("errors.Is(e, ErrLeaseHeld) = false")
	}
}

// TestLeaseHeldError_asResolvesThroughFmtWrapping verifies a wrapped
// LeaseHeldError is still discoverable via errors.As.
func TestLeaseHeldError_asResolvesThroughFmtWrapping(t *testing.T) {
	t.Parallel()

	original := &LeaseHeldError{RunID: "run-9", Owner: "owner-b"}
	wrapped := errors.Join(errors.New("claim failed"), original)

	var target *LeaseHeldError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As(wrapped, *LeaseHeldError) = false")
	}

	if target.RunID != "run-9" || target.Owner != "owner-b" {
		t.Errorf("target = %+v, want RunID=run-9 Owner=owner-b", target)
	}
}
