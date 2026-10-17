package serve

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
)

// mustProviders returns provider wiring for the boot tests: stub router,
// log, observability, db, and ratelimit. pingErr surfaces from the stub
// db's Ping. ZEVER_ENV=dev accepts the default deterministic dev key.
func mustProviders(t *testing.T, pingErr error) []container.Provider {
	t.Helper()
	t.Setenv("ZEVER_ENV", "dev")

	return wireStubProviders(t, pingErr, stubLogger{})
}

// mustProvidersNoEnv wires the stub providers without touching ZEVER_ENV,
// for the dev-key refusal test.
func mustProvidersNoEnv(t *testing.T, pingErr error) []container.Provider {
	t.Helper()

	return wireStubProviders(t, pingErr, stubLogger{})
}

// wireStubProviders builds the stub provider set with the given logger.
func wireStubProviders(t *testing.T, pingErr error, logger log.Logger) []container.Provider {
	t.Helper()

	return wireStubProvidersWithDB(t, logger, func(db.Options) (db.DB, error) {
		return &stubDB{pingErr: pingErr}, nil
	})
}

// wireStubProvidersWithDB builds the stub provider set with a custom db
// factory, for failure-injection tests.
func wireStubProvidersWithDB(t *testing.T, logger log.Logger, dbFactory func(db.Options) (db.DB, error)) []container.Provider {
	t.Helper()

	return []container.Provider{
		container.Provide(router.DefaultRef(), func(router.Options) (router.Router, error) {
			return newStubRouter(), nil
		}),
		container.Provide(log.DefaultRef(), func(log.Options) (log.Logger, error) {
			return logger, nil
		}),
		container.Provide(observability.DefaultRef(), func(observability.Options) (observability.Provider, error) {
			return stubProvider{}, nil
		}),
		container.Provide(db.DefaultRef(), dbFactory),
		container.Provide(ratelimit.DefaultRef(), func(ratelimit.Options) (ratelimit.Limiter, error) {
			return stubLimiter{}, nil
		}),
	}
}

// testOptions returns Options for the HTTP tests: ephemeral port, no gRPC,
// no pprof, empty literal env.
func testOptions() []Option {
	return []Option{
		WithPort("0"),
		WithEnvOverride(config.Env{}),
	}
}

// waitBoundAddr receives one bound listener address or fails the test,
// surfacing an early server error instead of hanging on it.
func waitBoundAddr(t *testing.T, ch <-chan string, errCh <-chan error) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	select {
	case addr := <-ch:
		return addr
	case err := <-errCh:
		t.Fatalf("server failed before listening: %v", err)

		return ""
	case <-ctx.Done():
		t.Fatal("timed out waiting for listener")

		return ""
	}
}

// dialAddr rewrites a bound address to a dialable loopback address.
func dialAddr(t *testing.T, bound string) string {
	t.Helper()

	_, port, err := net.SplitHostPort(bound)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", bound, err)
	}

	return net.JoinHostPort("127.0.0.1", port)
}

// getStatus performs one GET and returns the status code, draining the body.
func getStatus(t *testing.T, url string) int {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode
}

// serveOnce starts runServe in the background and returns the base URL plus
// a closer that cancels and waits for a clean shutdown.
func serveOnce(t *testing.T, providers []container.Provider, modsFn func(*container.Container) []Module, opts ...Option) (string, func()) {
	t.Helper()

	heard := make(chan string, 2)
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServe(ctx, providers, modsFn, append(opts, withListenHook(func(_, addr string) { heard <- addr }))...)
	}()

	base := "http://" + dialAddr(t, waitBoundAddr(t, heard, errCh))

	return base, func() {
		t.Helper()
		cancel()

		timeout, stop := context.WithTimeout(t.Context(), 10*time.Second)
		defer stop()

		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("runServe: %v", err)
			}
		case <-timeout.Done():
			t.Fatal("timed out waiting for shutdown")
		}
	}
}

// TestMainBootServesHealthzAndShutsDown boots a tiny stub app, proves
// /healthz answers 200, then cancels and proves shutdown is clean.
func TestMainBootServesHealthzAndShutsDown(t *testing.T) {
	mod := &stubModule{name: "app"}

	base, shutdown := serveOnce(t, mustProviders(t, nil), func(*container.Container) []Module {
		return []Module{mod}
	}, testOptions()...)
	defer shutdown()

	if got := getStatus(t, base+"/healthz"); got != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want %d", got, http.StatusOK)
	}

	if !mod.mounted.Load() {
		t.Fatal("module Mount was not called")
	}
}
