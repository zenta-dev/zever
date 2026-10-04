package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
)

// TestEdgeTranslate_emptyLocale proves an empty locale is rejected before
// any request is made.
func TestEdgeTranslate_emptyLocale(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, "http://example.invalid")
	t.Cleanup(func() { _ = a.Close() })

	_, err := a.Translate(t.Context(), "", "key", nil)

	var lnf *i18n.LocaleNotFoundError
	if !errors.As(err, &lnf) {
		t.Fatalf("Translate err = %v, want *LocaleNotFoundError", err)
	}
}

// TestEdgeLocales_empty proves a server returning an empty locale list is
// handled as a valid empty result.
func TestEdgeLocales_empty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"locales":[]}`))
	}))
	t.Cleanup(srv.Close)

	a := newTestAdapter(t, srv.URL)
	t.Cleanup(func() { _ = a.Close() })

	locales, err := a.Locales(t.Context())
	if err != nil {
		t.Fatalf("Locales: %v", err)
	}

	if len(locales) != 0 {
		t.Fatalf("Locales = %v, want empty", locales)
	}
}

// TestEdgeTranslate_nilArgs proves a nil args map is a valid cache key input.
func TestEdgeTranslate_nilArgs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"translations":{"k":"v"}}`))
	}))
	t.Cleanup(srv.Close)

	a := newTestAdapter(t, srv.URL)
	t.Cleanup(func() { _ = a.Close() })

	got, err := a.Translate(t.Context(), "en", "k", nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "v" {
		t.Fatalf("Translate = %q, want v", got)
	}
}
