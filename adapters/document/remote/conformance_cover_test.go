package remote

import (
	"testing"
)

// TestConformance is a live-creds stub: the shared document kit needs a live
// remote render endpoint, so conformance stays skipped here. Unit coverage
// with httptest doubles lives in remote_test.go. Never fake infra for
// conformance.
func TestConformance(t *testing.T) {
	t.Skip("needs live remote render endpoint; run in a live job with the endpoint reachable")
}
