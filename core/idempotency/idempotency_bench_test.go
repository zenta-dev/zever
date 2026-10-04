package idempotency

import (
	"testing"
	"time"
)

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Register(freshAdapter(), func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, err := Open(a, Options{})
		if err != nil {
			b.Fatalf("Open err = %v", err)
		}
		if err := s.Close(); err != nil {
			b.Fatalf("Close err = %v", err)
		}
	}
}

func BenchmarkValidateKey(b *testing.B) {
	key := "550e8400-e29b-41d4-a716-446655440000"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidateKey(key); err != nil {
			b.Fatalf("ValidateKey err = %v", err)
		}
	}
}

func BenchmarkFingerprintMatches(b *testing.B) {
	stored := []byte{1, 2, 3, 4, 5}
	incoming := []byte{1, 2, 3, 4, 5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !FingerprintMatches(stored, incoming) {
			b.Fatal("FingerprintMatches = false, want true")
		}
	}
}

func BenchmarkOptions_Validate(b *testing.B) {
	o := Options{TTL: time.Hour, Redis: RedisOptions{Addr: "localhost:6379", Prefix: "idem"}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkAdapter_String(b *testing.B) {
	a := Adapter("bench-adapter")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = a.String()
	}
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAdapter("bench-adapter")
	}
}

func BenchmarkStoreBegin(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	s, err := Open(a, Options{})
	if err != nil {
		b.Fatalf("Open err = %v", err)
	}
	b.Cleanup(func() { _ = s.Close() })

	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.Begin(ctx, "bench-key", BeginOptions{}); err != nil {
			b.Fatalf("Begin err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s, err := Open(a, Options{})
			if err != nil {
				b.Fatalf("Open err = %v", err)
			}
			_ = s.Close()
		}
	})
}
