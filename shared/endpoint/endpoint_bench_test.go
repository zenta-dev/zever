package endpoint

import "testing"

// BenchmarkValidateURLHTTPS measures the common https validation path.
func BenchmarkValidateURLHTTPS(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := ValidateURL("https://example.com/v1"); err != nil {
			b.Fatalf("ValidateURL() error = %v", err)
		}
	}
}

// BenchmarkValidateURLStrict measures validation with every policy enabled.
func BenchmarkValidateURLStrict(b *testing.B) {
	opts := []Option{WithRejectUserinfo(), WithRejectWhitespace(), WithRejectQueryFragment()}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := ValidateURL("https://example.com/v1", opts...); err != nil {
			b.Fatalf("ValidateURL() error = %v", err)
		}
	}
}

// BenchmarkValidateURLLoopback measures the loopback-http admission path.
func BenchmarkValidateURLLoopback(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := ValidateURL("http://127.0.0.1:8080", WithAllowLoopbackHTTP()); err != nil {
			b.Fatalf("ValidateURL() error = %v", err)
		}
	}
}

// BenchmarkIsLoopbackHost measures loopback host classification.
func BenchmarkIsLoopbackHost(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = IsLoopbackHost("127.0.0.1")
	}
}
