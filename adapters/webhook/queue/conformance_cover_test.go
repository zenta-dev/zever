package queue

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/webhook/webhooktest"
)

// TestConformanceQueue proves the queue-riding webhook passes the webhook
// kit over the in-memory queue. No live server is hit by the lifecycle
// kit; delivery fan-out is asserted in TestConformanceDeliveryQueue.
func TestConformanceQueue(t *testing.T) {
	t.Parallel()

	webhooktest.Conformance(t, func(t *testing.T) webhook.Webhook {
		t.Helper()

		return mustKitWebhook(t)
	})
}

// TestConformanceDeliveryQueue proves fan-out invocation through the queue
// adapter: Deliver enqueues without error and the background consumer
// reaches the httptest target.
func TestConformanceDeliveryQueue(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := mustKitWebhook(t)
	webhooktest.ConformanceDelivery(t, w, "kit.delivery", srv.URL+"/hook")
}

func mustKitWebhook(t *testing.T) webhook.Webhook {
	t.Helper()

	w, err := New(webhook.Options{
		QueueAdapter:        "memory",
		Timeout:             time.Second,
		AllowPrivateTargets: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })

	return w
}
