package grpcclient

import (
	"testing"
	"time"

	"google.golang.org/grpc/codes"
)

func BenchmarkOptionWithTimeout(b *testing.B) {
	opt := WithTimeout(5 * time.Second)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg config
		opt(&cfg)
	}
}

func BenchmarkOptionWithRetryPolicy(b *testing.B) {
	p := validRetryPolicy()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg config
		WithRetryPolicy(p)(&cfg)
	}
}

func BenchmarkOptionWithStaticResolver(b *testing.B) {
	addrs := map[string][]string{"users": {"127.0.0.1:50051", "127.0.0.2:50051"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg config
		WithStaticResolver(addrs)(&cfg)
	}
}

func BenchmarkOptionWithTLS(b *testing.B) {
	o := TLSOptions{ServerName: "example.com"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg config
		WithTLS(o)(&cfg)
	}
}

func BenchmarkOptionWithMetadata(b *testing.B) {
	md := map[string]string{"tenant": "acme"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var cfg config
		WithMetadata(md)(&cfg)
	}
}

func BenchmarkNewInsecure(b *testing.B) {
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		conn, err := New(ctx, "dns:///localhost:50051", WithInsecure())
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := conn.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

func BenchmarkNewWithRetryPolicy(b *testing.B) {
	ctx := b.Context()
	p := validRetryPolicy()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		conn, err := New(ctx, "dns:///localhost:50051", WithInsecure(), WithRetryPolicy(p))
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := conn.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

func BenchmarkServiceConfigJSONDefault(b *testing.B) {
	cfg := &config{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cfg.serviceConfigJSON(); err != nil {
			b.Fatalf("serviceConfigJSON() error = %v", err)
		}
	}
}

func BenchmarkServiceConfigJSONRetry(b *testing.B) {
	p := validRetryPolicy()
	cfg := &config{retry: &p}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := cfg.serviceConfigJSON(); err != nil {
			b.Fatalf("serviceConfigJSON() error = %v", err)
		}
	}
}

func BenchmarkRetryPolicyJSON(b *testing.B) {
	p := RetryPolicy{
		MaxAttempts:          3,
		InitialBackoff:       10 * time.Millisecond,
		MaxBackoff:           time.Second,
		BackoffMultiplier:    2,
		RetryableStatusCodes: []codes.Code{codes.Unavailable, codes.DeadlineExceeded},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := p.jsonRetryPolicy(); err != nil {
			b.Fatalf("jsonRetryPolicy() error = %v", err)
		}
	}
}
