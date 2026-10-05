package remote_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/adapters/i18n/remote"
	"github.com/zenta-dev/zever/core/i18n"
)

// TestEdgeTranslateArgsOrderCached checks that argument insertion order does
// not defeat the cache.
func TestEdgeTranslateArgsOrderCached(t *testing.T) {
	t.Parallel()

	f := &stubServer{values: map[string]string{"en\x00hello": "Hi"}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer ad.Close()

	ctx := t.Context()

	if _, err := ad.Translate(ctx, "en", "hello", map[string]string{"a": "1", "b": "2"}); err != nil {
		t.Fatalf("first Translate: %v", err)
	}

	if _, err := ad.Translate(ctx, "en", "hello", map[string]string{"b": "2", "a": "1"}); err != nil {
		t.Fatalf("second Translate: %v", err)
	}

	if n := f.translateHits.Load(); n != 1 {
		t.Fatalf("hits = %d, want 1 (args order must not split the cache)", n)
	}
}

// TestEdgeTranslateNilVsEmptyArgsCached checks that nil and empty arg maps are
// distinct cache entries and both succeed.
func TestEdgeTranslateNilVsEmptyArgsCached(t *testing.T) {
	t.Parallel()

	f := &stubServer{values: map[string]string{"en\x00hello": "Hi"}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer ad.Close()

	ctx := t.Context()

	if _, err := ad.Translate(ctx, "en", "hello", nil); err != nil {
		t.Fatalf("Translate(nil args): %v", err)
	}

	if _, err := ad.Translate(ctx, "en", "hello", map[string]string{}); err != nil {
		t.Fatalf("Translate(empty args): %v", err)
	}

	if n := f.translateHits.Load(); n != 2 {
		t.Fatalf("hits = %d, want 2 (nil and empty args are distinct keys)", n)
	}
}

// TestEdgeTranslateWhitespaceLocale checks that a whitespace-only locale is
// forwarded (unlike the embed adapter, which trims and rejects it).
func TestEdgeTranslateWhitespaceLocale(t *testing.T) {
	t.Parallel()

	f := &stubServer{values: map[string]string{}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer ad.Close()

	if _, err := ad.Translate(t.Context(), " ", "hello", nil); !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("Translate(whitespace locale) = %v, want ErrKeyNotFound", err)
	}
}

// TestEdgeLocalesEmpty checks an empty locales response.
func TestEdgeLocalesEmpty(t *testing.T) {
	t.Parallel()

	f := &stubServer{}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer ad.Close()

	got, err := ad.Locales(t.Context())
	if err != nil {
		t.Fatalf("Locales: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("Locales = %v, want empty", got)
	}
}

// TestEdgeConcurrentTranslateDistinctKeys exercises concurrent fetches of
// distinct keys on the goroutine-safe adapter.
func TestEdgeConcurrentTranslateDistinctKeys(t *testing.T) {
	t.Parallel()

	f := &stubServer{values: map[string]string{
		"en\x00k1": "v1",
		"en\x00k2": "v2",
		"en\x00k3": "v3",
		"en\x00k4": "v4",
	}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defer ad.Close()

	keys := []string{"k1", "k2", "k3", "k4"}

	var wg sync.WaitGroup

	for _, key := range keys {
		wg.Add(1)

		go func(key string) {
			defer wg.Done()

			for range 10 {
				if _, err := ad.Translate(t.Context(), "en", key, nil); err != nil {
					t.Errorf("Translate(%s): %v", key, err)
					return
				}
			}
		}(key)
	}

	wg.Wait()
}
