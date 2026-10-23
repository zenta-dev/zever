package scheduler

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalUnknownAdapterError covers the unknown-adapter typed error
// left uncovered when the adapter registry was removed: Open used to
// exercise it via unregistered-adapter lookups, so a direct unit test now
// pins the message constructor, sentinel unwrapping, and As round-trip.
func TestRegRemovalUnknownAdapterError(t *testing.T) {
	t.Parallel()

	err := UnknownAdapterError{Adapter: "cron"}

	msg := err.Error()
	if !strings.HasPrefix(msg, "scheduler: ") {
		t.Errorf("Error() = %q, want prefix %q", msg, "scheduler: ")
	}

	if !strings.Contains(msg, "cron") {
		t.Errorf("Error() = %q, want adapter name", msg)
	}

	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false")
	}

	var target UnknownAdapterError

	if !errors.As(err, &target) {
		t.Fatalf("errors.As(err, *UnknownAdapterError) = false")
	}

	if target.Adapter != "cron" {
		t.Errorf("Adapter = %q, want %q", target.Adapter, "cron")
	}

	if got := target.Unwrap(); got != ErrUnknownAdapter {
		t.Errorf("Unwrap() = %v, want ErrUnknownAdapter", got)
	}
}
