package cloudflare

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

func newBenchCloudflare(b *testing.B) *cloudflareAdapter {
	b.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"success":true,"errors":[],"messages":[]}`))
	}))
	b.Cleanup(srv.Close)

	opts := validOptions()
	opts.BaseURL = srv.URL
	c, err := New(opts)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}
	a, ok := c.(*cloudflareAdapter)
	if !ok {
		b.Fatalf("New() type = %T, want *cloudflareAdapter", c)
	}
	b.Cleanup(func() { _ = a.Close(b.Context()) })
	return a
}

func BenchmarkPurgeByURL(b *testing.B) {
	a := newBenchCloudflare(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := a.Purge(ctx, cdn.PurgeRequest{URLs: []string{"https://example.com/style.css"}}); err != nil {
			b.Fatalf("Purge(): %v", err)
		}
	}
}

func BenchmarkPurgeByTag(b *testing.B) {
	a := newBenchCloudflare(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := a.Purge(ctx, cdn.PurgeRequest{Tags: []string{"tag1"}}); err != nil {
			b.Fatalf("Purge(): %v", err)
		}
	}
}

func BenchmarkPurgeAll(b *testing.B) {
	a := newBenchCloudflare(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := a.Purge(ctx, cdn.PurgeRequest{All: true}); err != nil {
			b.Fatalf("Purge(): %v", err)
		}
	}
}
