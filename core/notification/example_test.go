package notification_test

import (
	"context"
	"errors"
	"io"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
)

// ExampleOpen delivers one push notification through the log adapter.
func ExampleOpen() {
	if err := notification.Register(notification.Log, func(o notification.Options) (notification.Notifier, error) {
		return notificationlog.NewWithWriter(o, io.Discard)
	}); err != nil && !errors.Is(err, notification.ErrDuplicate) {
		return
	}

	n, err := notification.Open(notification.Log, notification.Options{})
	if err != nil {
		return
	}
	defer func() { _ = n.Close() }()

	msg := notification.NewNotification("device-token", notification.ChannelPush, "hello")

	_ = n.Notify(context.Background(), &msg)
}
