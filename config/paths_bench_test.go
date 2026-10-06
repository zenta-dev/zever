package config

import "testing"

// BenchmarkIsSensitiveKey measures the key classifier over the mix of
// sensitive and non-sensitive spellings seen in option maps.
func BenchmarkIsSensitiveKey(b *testing.B) {
	keys := []string{"password", "api_key", "addr", "MaxRetries", "webhook_secret", "region"}

	b.ReportAllocs()
	for b.Loop() {
		for _, k := range keys {
			_ = isSensitiveKey(k)
		}
	}
}

// BenchmarkRedactStringSlice measures in-place header-list redaction over a
// three-entry list (one secret, two plain), including the copy the caller
// owns.
func BenchmarkRedactStringSlice(b *testing.B) {
	src := []string{"X-Auth-Token: abc", "Content-Type: json", "Cookie: sid=1"}

	b.ReportAllocs()
	for b.Loop() {
		s := make([]string, len(src))
		copy(s, src)
		redactStringSlice(s)
	}
}

// BenchmarkNormEqualFold measures the allocation-free normalized comparison
// behind env-var field matching.
func BenchmarkNormEqualFold(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if !normEqualFold("Max_Retries", "maxretries") {
			b.Fatal("normEqualFold mismatch")
		}
	}
}
