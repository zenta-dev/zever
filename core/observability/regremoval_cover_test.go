package observability

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers the registry typed errors left uncovered
// when the adapter registry was removed: Register/Open used to exercise
// them via duplicate/unknown/invalid adapter paths, so direct unit tests
// now pin the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "otel"}, ErrDuplicateAdapter, "otel", &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "otel"}, ErrUnknownAdapter, "otel", &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, "bogus", &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad endpoint"}, ErrInvalidOptions, "bad endpoint", &InvalidOptionsError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "observability: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "observability: ")
			}

			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tc.wantSub)
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}

// TestRegRemovalAttributeValues covers the isAttributeValue marker methods,
// which carry no statements beyond the method itself and are only exercised
// by constructing each value type.
func TestRegRemovalAttributeValues(t *testing.T) {
	t.Parallel()

	StringValue{Value: "v"}.isAttributeValue()
	Int64Value{Value: 1}.isAttributeValue()
	Float64Value{Value: 1.5}.isAttributeValue()
	BoolValue{Value: true}.isAttributeValue()
}
