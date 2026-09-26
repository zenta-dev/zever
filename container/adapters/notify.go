package adapters

import (
	"github.com/zenta-dev/zever/notification"
	notificationfcm "github.com/zenta-dev/zever/notification/fcm"
	notificationtwilio "github.com/zenta-dev/zever/notification/twilio"
)

// RegisterNotify registers the provider-backed notification adapters the
// core container no longer imports: fcm (firebase-admin SDK) and twilio
// (twilio-go SDK). The log adapter stays wired by the container.
// Registration only fills factory maps; it performs no I/O. Duplicate
// registrations are ignored, so calling RegisterNotify more than once (or
// alongside RegisterAll) is safe.
func RegisterNotify() {
	_ = notification.Register(notification.FCM, notificationfcm.New)
	_ = notification.Register(notification.Twilio, notificationtwilio.New)
}
