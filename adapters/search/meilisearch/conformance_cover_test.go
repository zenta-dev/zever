package meilisearch

import (
	"testing"
)

// TestConformance is a live-creds stub: the shared search kit needs a live
// Meilisearch host plus API key, so conformance stays skipped here. Unit
// coverage with httptest doubles lives in meilisearch_test.go. Never fake
// infra for conformance.
func TestConformance(t *testing.T) {
	t.Skip("needs live Meilisearch (host + API key); set MEILISEARCH_HOST/MEILISEARCH_API_KEY in a live job")
}
