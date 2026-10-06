package grpcclient

import (
	"testing"

	"google.golang.org/grpc/codes"
)

// BenchmarkRetryableCodeName measures the status-code to service-config name
// mapping on the retry-policy hot path.
func BenchmarkRetryableCodeName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := retryableCodeName(codes.Unavailable); got != "UNAVAILABLE" {
			b.Fatalf("retryableCodeName = %q", got)
		}
	}
}

// BenchmarkCredentialsBare measures TLS credential construction without any
// files involved.
func BenchmarkCredentialsBare(b *testing.B) {
	o := TLSOptions{ServerName: "example.com"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := o.credentials(); err != nil {
			b.Fatalf("credentials() error = %v", err)
		}
	}
}

// BenchmarkStartSpanNoop measures the provider-less span start on the
// per-RPC hot path.
func BenchmarkStartSpanNoop(b *testing.B) {
	cfg := &config{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, span := cfg.startSpan(ctx, "/svc/Method")
		span.End()
	}
}

// BenchmarkTransportCredentialsInsecure measures the insecure credential
// resolution on the dial hot path.
func BenchmarkTransportCredentialsInsecure(b *testing.B) {
	cfg := &config{insecure: true}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cfg.transportCredentials(); err != nil {
			b.Fatalf("transportCredentials() error = %v", err)
		}
	}
}
