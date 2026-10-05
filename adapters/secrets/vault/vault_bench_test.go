package vault

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// newBenchSecrets opens a driver backed by an in-process stub Vault server and
// seeds one secret for the Get hot path.
func newBenchSecrets(b *testing.B) secrets.Secrets {
	b.Helper()

	stub := &stubVault{store: map[string]string{}, token: "bench-token", mount: "secret"}
	srv := httptest.NewServer(stub.handler())
	b.Cleanup(srv.Close)

	s, err := New(Options{Addr: srv.URL, Token: "bench-token", Mount: "secret"})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	if err := s.Set(b.Context(), "bench", []byte("value")); err != nil {
		b.Fatalf("Set(): %v", err)
	}

	return s
}

// BenchmarkGet measures a full KVv2 read round trip against the in-process
// stub: request build, HTTP round trip, and base64 decode.
func BenchmarkGet(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Get(ctx, "bench"); err != nil {
			b.Fatalf("Get(): %v", err)
		}
	}
}

// BenchmarkSet measures the KVv2 write round trip, including base64 encoding.
func BenchmarkSet(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()
	value := []byte("bench-value")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Set(ctx, "bench", value); err != nil {
			b.Fatalf("Set(): %v", err)
		}
	}
}

// BenchmarkList measures the metadata list round trip.
func BenchmarkList(b *testing.B) {
	s := newBenchSecrets(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.List(ctx); err != nil {
			b.Fatalf("List(): %v", err)
		}
	}
}

// BenchmarkDelete measures a KVv2 metadata delete round trip against a stub
// that always accepts the delete (Vault returns 404 for missing keys, so the
// stub keeps the hot path free of seed writes).
func BenchmarkDelete(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	b.Cleanup(srv.Close)

	s, err := New(Options{Addr: srv.URL, Token: "bench-token", Mount: "secret"})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Delete(ctx, "bench"); err != nil {
			b.Fatalf("Delete(): %v", err)
		}
	}
}
