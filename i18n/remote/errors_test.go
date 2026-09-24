package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/i18n"
)

func TestNewEmptyEndpoint(t *testing.T) {
	t.Parallel()

	_, err := New(i18n.Options{})
	if err == nil {
		t.Fatal("New() = nil error, want endpoint required")
	}
	if !errors.Is(err, i18n.ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
	var ioe *i18n.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Errorf("errors.As(err, InvalidOptionsError) = false (err = %T %v)", err, err)
	}
}

func TestRemoteStatusIs(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	a, err := New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: srv.URL, AllowInsecure: true}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	if _, err := a.Translate(t.Context(), "en", "hi", nil); !errors.Is(err, i18n.ErrRemoteError) {
		t.Errorf("Translate err = %v, want ErrRemoteError", err)
	}
}
