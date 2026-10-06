package notificationtest_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/notification/notificationtest"
)

// stubSMSOnly is an SMS-channel-scoped notifier: push delivery reports
// ErrChannelNotSupported while SMS delivery succeeds. It exercises the
// kit's SMS-fallback branch in conformanceHappyPath, which the
// push-capable log adapter never reaches.
type stubSMSOnly struct{}

func (stubSMSOnly) Notify(_ context.Context, n *notification.Notification) error {
	if n == nil {
		return notification.ErrNilNotification
	}
	if err := n.Validate(); err != nil {
		return err
	}
	if n.Channel != notification.ChannelSMS {
		return notification.ErrChannelNotSupported
	}
	return nil
}

func (stubSMSOnly) Close() error { return nil }

// TestConformanceSMSOnly proves the kit passes against an SMS-only
// notifier, covering the push-unsupported SMS fallback path.
func TestConformanceSMSOnly(t *testing.T) {
	t.Parallel()

	notificationtest.Conformance(t, func(t *testing.T) notification.Notifier {
		t.Helper()

		return stubSMSOnly{}
	})
}

// TestConformanceSMSOnlyCloseIdempotent proves the SMS-only stub honors
// the idempotent-Close contract the kit asserts.
func TestConformanceSMSOnlyCloseIdempotent(t *testing.T) {
	t.Parallel()

	n := stubSMSOnly{}
	if err := n.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close() second error = %v, want nil", err)
	}
}
