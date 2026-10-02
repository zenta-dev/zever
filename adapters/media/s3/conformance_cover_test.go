package s3

import (
	"testing"
)

// TestConformance is a live-creds stub: the shared media kit needs an
// S3-compatible endpoint plus bucket credentials, so conformance stays
// skipped here. Unit coverage with fakes lives in s3_test.go. Never fake
// infra for conformance.
func TestConformance(t *testing.T) {
	t.Skip("needs live S3-compatible endpoint with bucket credentials; run in a live job")
}
