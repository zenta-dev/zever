package env

import (
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// newBenchSecrets opens a prefixed env adapter and seeds one variable so the
// Get hot path takes the hit branch.
func newBenchSecrets(b *testing.B) secrets.Secrets {
	b.Helper()

	b.Setenv("TBENCH_GREETING", "hello")

	s, err := New(Options{Prefix: "TBENCH_"})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	return s
}

// BenchmarkGet measures a single prefixed environment lookup (os.LookupEnv).
func BenchmarkGet(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := s.Get(ctx, "GREETING"); err != nil {
			b.Fatalf("Get(): %v", err)
		}
	}
}

// BenchmarkGetParallel measures prefixed lookups under concurrent load;
// the adapter is stateless apart from its immutable prefix.
func BenchmarkGetParallel(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := s.Get(ctx, "GREETING"); err != nil {
				b.Errorf("Get(): %v", err)
				return
			}
		}
	})
}

// BenchmarkList measures enumerating the process environment and filtering to
// the adapter's prefix boundary.
func BenchmarkList(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := s.List(ctx); err != nil {
			b.Fatalf("List(): %v", err)
		}
	}
}
