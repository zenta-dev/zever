package fcm

import (
	"context"
	"testing"

	"firebase.google.com/go/v4/messaging"

	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/notification/notificationtest"
)

// TestConformanceFCM proves the FCM notifier passes the notification kit
// with an injected fake send: no live network is used and no credentials
// are needed.
func TestConformanceFCM(t *testing.T) {
	t.Parallel()

	notificationtest.Conformance(t, func(t *testing.T) notification.Notifier {
		t.Helper()

		n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
			return "projects/kit/messages/m", nil
		}}

		t.Cleanup(func() { _ = n.Close() })

		return n
	})
}
