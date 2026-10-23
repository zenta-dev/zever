package idempotency

import (
	"testing"
	"time"
)

func BenchmarkValidateKey(b *testing.B) {
	key := "550e8400-e29b-41d4-a716-446655440000"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
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
	for b.Loop() {
		if !FingerprintMatches(stored, incoming) {
			b.Fatal("FingerprintMatches = false, want true")
		}
	}
}

func BenchmarkOptions_Validate(b *testing.B) {
	o := Options{TTL: time.Hour, Redis: RedisOptions{Addr: "localhost:6379", Prefix: "idem"}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

