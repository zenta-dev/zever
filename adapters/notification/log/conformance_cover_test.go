package log_test

import (
	"bytes"
	"testing"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/notification/notificationtest"
)

// TestConformanceLog proves the log notifier passes the notification kit.
func TestConformanceLog(t *testing.T) {
	t.Parallel()

	notificationtest.Conformance(t, func(t *testing.T) notification.Notifier {
		t.Helper()

		n, err := notificationlog.NewWithWriter(notification.Options{}, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = n.Close() })

		return n
	})
}
