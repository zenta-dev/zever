package ai

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers the duplicate-registration typed error
// left uncovered when the adapter registry was removed: registries used to
// exercise it via duplicate Register calls, so direct unit tests now pin
// the message constructor, sentinel unwrapping, and As round-trip.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	err := DuplicateAdapterError{Adapter: "openai"}

	msg := err.Error()
	if !strings.HasPrefix(msg, "ai: ") {
		t.Errorf("Error() = %q, want prefix %q", msg, "ai: ")
	}

	if !strings.Contains(msg, "openai") {
		t.Errorf("Error() = %q, want adapter name", msg)
	}

	if !errors.Is(err, ErrDuplicateAdapter) {
		t.Errorf("errors.Is(err, ErrDuplicateAdapter) = false")
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("errors.Is(err, ErrDuplicate) = false")
	}

	var target DuplicateAdapterError

	if !errors.As(err, &target) {
		t.Fatalf("errors.As(err, *DuplicateAdapterError) = false")
	}

	if target.Adapter != "openai" {
		t.Errorf("Adapter = %q, want %q", target.Adapter, "openai")
	}

	if !errors.Is(target, ErrDuplicateAdapter) {
		t.Errorf("Unwrap() = %v, want ErrDuplicateAdapter", target.Unwrap())
	}

	var aliasTarget DuplicateError

	if !errors.As(err, &aliasTarget) {
		t.Errorf("errors.As(err, *DuplicateError) = false")
	}
}
