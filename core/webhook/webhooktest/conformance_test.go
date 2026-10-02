package webhooktest_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/webhook/webhooktest"
)

// TestConformanceHTTP proves the kit passes against the http adapter.
func TestConformanceHTTP(t *testing.T) {
	t.Parallel()

	webhooktest.Conformance(t, func(t *testing.T) webhook.Webhook {
		t.Helper()

		return mustNew(t, webhook.Options{AllowPrivateTargets: true})
	})
}

// TestConformanceDeliveryHTTP proves live fan-out against the http adapter.
func TestConformanceDeliveryHTTP(t *testing.T) {
	t.Parallel()

	var received atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := mustNew(t, webhook.Options{AllowPrivateTargets: true})
	webhooktest.ConformanceDelivery(t, w, "kit.delivery", srv.URL+"/hook")

	if got := received.Load(); got != 1 {
		t.Errorf("server received %d deliveries, want 1", got)
	}
}

func mustNew(t *testing.T, opts webhook.Options) webhook.Webhook {
	t.Helper()

	w, err := webhookhttp.New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })

	return w
}
