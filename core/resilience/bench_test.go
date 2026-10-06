package resilience_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func benchAdapter(b *testing.B) resilience.Adapter {
	b.Helper()

	a := freshAdapter()
	if err := resilience.Register(a, func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := resilience.Open(a, resilience.Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

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
