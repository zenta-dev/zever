package notification_test

import (
	"context"
	"io"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
)

// Example delivers one push notification through the log adapter.
func Example() {
	n, err := notificationlog.NewWithWriter(notification.Options{}, io.Discard)
	if err != nil {
		return
	}
	defer func() { _ = n.Close() }()

	msg := notification.NewNotification("device-token", notification.ChannelPush, "hello")

	_ = n.Notify(context.Background(), &msg)
}
