package serve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/router"
)

// TestReadyzReportsReadyWhenDBHealthy proves /readyz answers 200 when the
// database pings cleanly.
func TestReadyzReportsReadyWhenDBHealthy(t *testing.T) {
	base, shutdown := serveOnce(t, mustProviders(t, nil), func(*container.Container) []Module {
		return nil
	}, testOptions()...)
	defer shutdown()

	if got := getStatus(t, base+"/readyz"); got != http.StatusOK {
		t.Fatalf("GET /readyz = %d, want %d", got, http.StatusOK)
	}
}

// TestReadyzReportsNotReadyWhenDBDown proves /readyz answers 503 when the
// database ping fails.
func TestReadyzReportsNotReadyWhenDBDown(t *testing.T) {
	base, shutdown := serveOnce(t, mustProviders(t, errors.New("db: ping: connection refused")), func(*container.Container) []Module {
		return nil
	}, testOptions()...)
	defer shutdown()

	if got := getStatus(t, base+"/readyz"); got != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz = %d, want %d", got, http.StatusServiceUnavailable)
	}
}

// TestSecurityHeadersShipByDefault proves baseline response headers land on
// every response, and HSTS stays off without TLS.
func TestSecurityHeadersShipByDefault(t *testing.T) {
	base, shutdown := serveOnce(t, mustProviders(t, nil), func(*container.Container) []Module {
		return nil
	}, testOptions()...)
	defer shutdown()

	resp := getResponse(t, base+"/healthz")
	defer func() { _ = resp.Body.Close() }()

	for _, want := range []string{
		"X-Content-Type-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"X-Frame-Options",
	} {
		if resp.Header.Get(want) == "" {
			t.Errorf("header %s missing", want)
		}
	}

	if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q without TLS, want absent", got)
	}
}

// TestRequestBodyLimitRejectsOversizedBodies proves a body past
// DefaultMaxRequestBodyBytes fails the handler read instead of buffering.
func TestRequestBodyLimitRejectsOversizedBodies(t *testing.T) {
	mod := &stubModule{name: "app", mountFn: func(r router.Router, _ *container.Container) error {
		r.Handle("POST", "/upload", func(w http.ResponseWriter, req *http.Request) {
			if _, err := io.ReadAll(req.Body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte("too large"))

				return
			}

			w.WriteHeader(http.StatusOK)
		})

		return nil
	}}

	base, shutdown := serveOnce(t, mustProviders(t, nil), func(*container.Container) []Module {
		return []Module{mod}
	}, testOptions()...)
	defer shutdown()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	big := bytes.NewReader(make([]byte, DefaultMaxRequestBodyBytes+1))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/upload", big)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /upload: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusBadRequest || string(body) != "too large" {
		t.Fatalf("POST /upload = %d %q, want 400 %q", resp.StatusCode, body, "too large")
	}
}

// TestReadyzLogsMigrateHintOnMissingTable proves a missing-table readiness
// failure logs the migrate command server-side.
func TestReadyzLogsMigrateHintOnMissingTable(t *testing.T) {
	t.Setenv("ZEVER_ENV", "dev")

	logger := &recordLogger{}

	base, shutdown := serveOnce(t, wireStubProviders(t, errNoSuchTable, logger), func(*container.Container) []Module {
		return nil
	}, testOptions()...)
	defer shutdown()

	if got := getStatus(t, base+"/readyz"); got != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz = %d, want %d", got, http.StatusServiceUnavailable)
	}

	if !logger.warnsContaining("zever db migrate") {
		t.Fatal("readiness warning lacks the `zever db migrate` hint")
	}
}

// errNoSuchTable is the missing-table failure for the migrate-hint tests.
var errNoSuchTable = errors.New("db: query users: no such table: users")

// getResponse performs one GET and returns the response for header checks.
func getResponse(t *testing.T, url string) *http.Response {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	_, _ = io.Copy(io.Discard, resp.Body)

	return resp
}
