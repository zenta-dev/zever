package httpclient

import (
	"bytes"
	"net"
	"testing"
	"time"
)

// BenchmarkReadLimited measures reading a small body within the limit.
func BenchmarkReadLimited(b *testing.B) {
	data := bytes.Repeat([]byte("x"), 1024)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ReadLimited(b.Context(), bytes.NewReader(data), 4096); err != nil {
			b.Fatalf("ReadLimited() error = %v", err)
		}
	}
}

// BenchmarkIsPrivateIP measures classifying an IPv4 address.
func BenchmarkIsPrivateIP(b *testing.B) {
	ip := net.ParseIP("10.1.2.3")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = IsPrivateIP(ip)
	}
}

// BenchmarkNewClient measures constructing a TLS-floored client.
func BenchmarkNewClient(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = NewClient(time.Second)
	}
}

// BenchmarkSafeDialContext measures the closure construction of the dial guard.
func BenchmarkSafeDialContext(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SafeDialContext(false)
	}
}

// BenchmarkNewSafeClient measures constructing the hardened client with the
// dial guard and no-redirect policy.
func BenchmarkNewSafeClient(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = NewSafeClient(time.Second, false)
	}
}
