package ai

import (
	"testing"
	"time"
)

// benchAdapter registers a stub AI once and returns its adapter so Open can
// be measured without paying the (one-shot) registration cost.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{BaseURL: "https://api.openai.com/v1", Timeout: 30 * time.Second}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
