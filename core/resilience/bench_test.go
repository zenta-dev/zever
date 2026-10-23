package resilience_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func BenchmarkOptionsValidate(b *testing.B) {
	opts := resilience.Default()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkDo(b *testing.B) {
	g := &stubGuard{name: "dep"}
	fn := func(context.Context) (int, error) { return 1, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := resilience.Do(b.Context(), g, fn); err != nil {
			b.Fatalf("Do error = %v", err)
		}
	}
}
