package firebase

import (
	"testing"

	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/flag/flagtest"
)

// TestFirebaseConformance proves the firebase adapter honors the
// flag.Flag contract via the shared conformance kit.
//
// Currently skipped: the adapter needs live Firebase credentials
// (project ID + service-account key) and network access to Google.
// The kit's fallback assertions match the static adapter's; seeded
// targeting coverage lives in the adapter's own tests. Re-enable
// with test credentials on loopback.
func TestFirebaseConformance(t *testing.T) {
	t.Skip("needs live Firebase credentials and network")

	flagtest.Conformance(t, func(t *testing.T) flag.Flag {
		t.Helper()

		f, err := New(flag.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = f.Close() })

		return f
	})
}
