package qdrant

import (
	"testing"
)

// TestConformance is a live-creds stub: the shared vectorstore kit needs a
// live Qdrant URL, so conformance stays skipped here. Unit coverage with
// fakes lives in qdrant_test.go. Never fake infra for conformance.
func TestConformance(t *testing.T) {
	t.Skip("needs live Qdrant (URL); run in a live job with Qdrant reachable")
}
