package fcm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/notification"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	if got, want := ErrNotConfigured.Error(), "fcm: not configured"; got != want {
		t.Errorf("sentinel message = %q, want %q", got, want)
	}
}

func TestNotConfiguredIs(t *testing.T) {
	t.Parallel()

	n := &notification.Notification{
		Target:  "device-token-123",
		Channel: notification.ChannelPush,
		Title:   "Hello",
		Body:    "hello",
	}

	if err := (&notifier{}).Notify(t.Context(), n); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("Notify err = %v, want ErrNotConfigured", err)
	}
}
