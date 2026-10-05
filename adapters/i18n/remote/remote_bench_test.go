package remote_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/adapters/i18n/remote"
	"github.com/zenta-dev/zever/core/i18n"
)

var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run so
// Register never collides with a previously registered factory.
func benchFreshAdapter() i18n.Adapter {
	return i18n.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

// stubI18n is a no-op i18n.I18n used to isolate registry benchmarks.
type stubI18n struct{}

func (stubI18n) Translate(context.Context, string, string, map[string]string) (string, error) {
	return "", nil
}
func (stubI18n) Locales(context.Context) ([]string, error) { return nil, nil }
func (stubI18n) Close() error                              { return nil }

// benchRemote builds a remote adapter over a stub HTTP server (loopback, no
// real network) that echoes the requested key.
func benchRemote(b *testing.B) i18n.I18n {
	b.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key string `json:"key"`
		}

		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"translations": map[string]string{req.Key: "v"}})
	})

	mux.HandleFunc("/locales", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"locales": []string{"en", "de"}})
	})

	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)

	ad, err := remote.New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: srv.URL, AllowInsecure: true}})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = ad.Close() })

	return ad
}

// BenchmarkTranslateCached measures the in-memory cache-hit path.
func BenchmarkTranslateCached(b *testing.B) {
	ad := benchRemote(b)
	ctx := b.Context()

	if _, err := ad.Translate(ctx, "en", "hello", nil); err != nil {
		b.Fatalf("warm Translate() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ad.Translate(ctx, "en", "hello", nil); err != nil {
			b.Fatalf("Translate() error = %v", err)
		}
	}
}

// BenchmarkTranslateNetwork measures a full round trip for a fresh key.
func BenchmarkTranslateNetwork(b *testing.B) {
	ad := benchRemote(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	var i atomic.Int64

	for b.Loop() {
		key := fmt.Sprintf("k-%d", i.Add(1))

		if _, err := ad.Translate(ctx, "en", key, nil); err != nil {
			b.Fatalf("Translate() error = %v", err)
		}
	}
}

// BenchmarkLocalesCached measures the cached locales-list path.
func BenchmarkLocalesCached(b *testing.B) {
	ad := benchRemote(b)
	ctx := b.Context()

	if _, err := ad.Locales(ctx); err != nil {
		b.Fatalf("warm Locales() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := ad.Locales(ctx); err != nil {
			b.Fatalf("Locales() error = %v", err)
		}
	}
}

// BenchmarkRegister measures registering a factory into the i18n registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		err := i18n.Register(benchFreshAdapter(), func(i18n.Options) (i18n.I18n, error) {
			return stubI18n{}, nil
		})
		if err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()

	err := i18n.Register(a, func(i18n.Options) (i18n.I18n, error) { return stubI18n{}, nil })
	if err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		ad, openErr := i18n.Open(a, i18n.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := ad.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
