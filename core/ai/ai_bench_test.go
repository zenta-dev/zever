package ai

import (
	"testing"
	"time"
)

// BenchmarkOptionsValidate measures Options validation for an AI backend.
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
