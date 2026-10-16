package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchUpgradeURLLimit proves a response larger than the cap is refused
// instead of being buffered whole, and one at the cap is returned intact.
func TestFetchUpgradeURLLimit(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	t.Cleanup(srv.Close)

	got, err := fetchUpgradeURLLimit(t.Context(), srv.URL, 100)
	if err != nil {
		t.Fatalf("fetch at the cap: %v", err)
	}

	if len(got) != 100 {
		t.Errorf("len = %d, want 100", len(got))
	}

	if _, err := fetchUpgradeURLLimit(t.Context(), srv.URL, 99); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("over the cap: err = %v, want an exceeds-limit error", err)
	}
}
