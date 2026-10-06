package webhook

import (
	"net"
	"testing"
)

// benchWebhookAdapter registers a stub webhook once and returns its adapter so
// Open can be measured without the one-shot registration cost.
func benchWebhookAdapter(b *testing.B) Adapter {
	b.Helper()

	a := freshAdapter()
	if err := Register(a, func(Options) (Webhook, error) { return &stubWebhook{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchWebhookAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{Timeout: 5, MaxRetries: 3, ReplayTolerance: 300}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}

func BenchmarkIsPrivateIP(b *testing.B) {
	ip := net.ParseIP("10.1.2.3")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !IsPrivateIP(ip) {
			b.Fatal("IsPrivateIP(10.1.2.3) = false")
		}
	}
}

func BenchmarkValidateTargetSyntax(b *testing.B) {
	target := "https://hooks.example.com/v1/events"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateTargetSyntax(target); err != nil {
			b.Fatalf("ValidateTargetSyntax(%q) error = %v", target, err)
		}
	}
}

func BenchmarkNewSafeClient(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if c := NewSafeClient(5, false); c == nil {
			b.Fatal("NewSafeClient returned nil")
		}
	}
}
