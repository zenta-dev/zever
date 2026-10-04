package redisopt

import "testing"

// BenchmarkValidateAddr measures validating a host:port address.
func BenchmarkValidateAddr(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := ValidateAddr("localhost:6379"); err != nil {
			b.Fatalf("ValidateAddr() error = %v", err)
		}
	}
}

// BenchmarkValidatePrefix measures validating a token-character prefix.
func BenchmarkValidatePrefix(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := ValidatePrefix("session_v1"); err != nil {
			b.Fatalf("ValidatePrefix() error = %v", err)
		}
	}
}

// BenchmarkIsTokenChar measures token-character classification.
func BenchmarkIsTokenChar(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = IsTokenChar('a')
	}
}

// BenchmarkRedactAddr measures masking embedded credentials.
func BenchmarkRedactAddr(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = RedactAddr("redis://bob:s3cret@h:6379")
	}
}
