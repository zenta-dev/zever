package orm

import (
	"errors"
	"fmt"
	"testing"
)

// BenchmarkIsRetryablePlain measures the negative fast path: a plain error
// must be inspected and rejected without allocation.
func BenchmarkIsRetryablePlain(b *testing.B) {
	err := errors.New("connection reset")

	b.ReportAllocs()

	for b.Loop() {
		if IsRetryable(err) {
			b.Fatal("IsRetryable(plain) = true, want false")
		}
	}
}

// BenchmarkIsRetryableWrapped measures the positive path: an error wrapping
// ErrRetryable is recognized through the chain.
func BenchmarkIsRetryableWrapped(b *testing.B) {
	err := fmt.Errorf("orm: exec: %w", ErrRetryable)

	b.ReportAllocs()

	for b.Loop() {
		if !IsRetryable(err) {
			b.Fatal("IsRetryable(wrapped ErrRetryable) = false, want true")
		}
	}
}
