package twilio

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/notification/notificationtest"
)

// TestConformanceTwilio proves the Twilio notifier passes the notification
// kit with its HTTP transport rewritten at an httptest server: no live
// network is used.
func TestConformanceTwilio(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SMkit","status":"queued"}`))
	}))
	t.Cleanup(srv.Close)

	notificationtest.Conformance(t, func(t *testing.T) notification.Notifier {
		t.Helper()

		return newTestNotifier(t, srv)
	})
}
