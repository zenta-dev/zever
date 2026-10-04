package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/webhook"
)

// newBenchAdapter opens an HTTP webhook adapter pointed at an in-process
// target that always returns 200, with private targets allowed so delivery
// skips DNS-based validation.
func newBenchAdapter(b *testing.B) *adapter {
	b.Helper()

	w, err := New(webhook.Options{AllowPrivateTargets: true, MaxRetries: 1, Timeout: 5 * time.Second})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	a, ok := w.(*adapter)
	if !ok {
		b.Fatalf("New() type = %T, want *adapter", w)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	b.Cleanup(srv.Close)

	if err := a.Register(b.Context(), "evt", srv.URL, "bench-secret"); err != nil {
		b.Fatalf("Register(): %v", err)
	}

	return a
}

// BenchmarkDeliver measures a full synchronous delivery to one target:
// request build, HMAC signature header, POST, and body drain.
func BenchmarkDeliver(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()
	payload := []byte(`{"event":"bench"}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := a.Deliver(ctx, "evt", payload); err != nil {
			b.Fatalf("Deliver(): %v", err)
		}
	}
}

// BenchmarkSignAt measures the deterministic HMAC-SHA256 envelope used for
// delivery signatures.
func BenchmarkSignAt(b *testing.B) {
	payload := []byte(`{"event":"bench"}`)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if sig := signAt("bench-secret", payload, 1700000000); sig == "" {
			b.Fatal("signAt() returned empty signature")
		}
	}
}
