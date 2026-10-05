package redis

import (
	"testing"
	"time"
)

// BenchmarkRedisKey measures prefixing an idempotency key.
func BenchmarkRedisKey(b *testing.B) {
	s := &store{prefix: "idem:"}

	b.ReportAllocs()

	for b.Loop() {
		_ = s.redisKey("bench-key")
	}
}

// BenchmarkEncodePending measures the pending wire-record builder.
func BenchmarkEncodePending(b *testing.B) {
	fp := []byte("fingerprint")

	b.ReportAllocs()

	for b.Loop() {
		_ = encodePending(fp)
	}
}

// BenchmarkEncodeDone measures the completed wire-record builder.
func BenchmarkEncodeDone(b *testing.B) {
	fp := []byte("fingerprint")
	result := []byte("result-payload")

	b.ReportAllocs()

	for b.Loop() {
		_ = encodeDone(fp, result)
	}
}

// BenchmarkDecode measures splitting a completed wire record.
func BenchmarkDecode(b *testing.B) {
	raw := encodeDone([]byte("fingerprint"), []byte("result-payload"))

	b.ReportAllocs()

	for b.Loop() {
		if _, _, _, err := decode(raw); err != nil {
			b.Fatalf("decode failed: %v", err)
		}
	}
}

// BenchmarkCheckFingerprint measures the fingerprint size guard.
func BenchmarkCheckFingerprint(b *testing.B) {
	fp := []byte("fingerprint")

	b.ReportAllocs()

	for b.Loop() {
		if err := checkFingerprint(fp); err != nil {
			b.Fatalf("checkFingerprint failed: %v", err)
		}
	}
}

// BenchmarkTTLModeVal measures mapping a duration to a Redis SET mode and
// value, the only per-call formatting the Lua scripts receive.
func BenchmarkTTLModeVal(b *testing.B) {
	ttls := []time.Duration{time.Nanosecond, 1500 * time.Microsecond, 500 * time.Millisecond, 90 * time.Second}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_, _ = ttlModeVal(ttls[i%len(ttls)])
		i++
	}
}

// BenchmarkAsBool measures normalizing a Lua table element to a bool.
func BenchmarkAsBool(b *testing.B) {
	values := []any{true, int64(1), int64(0), "x"}

	b.ReportAllocs()

	i := 0

	for b.Loop() {
		_ = asBool(values[i%len(values)])
		i++
	}
}

// BenchmarkRedactAddr measures credential masking for error messages.
func BenchmarkRedactAddr(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = redactAddr("redis://user:secret@localhost:6379")
	}
}
