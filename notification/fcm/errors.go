package fcm

import "errors"

// ErrNotConfigured is returned when the FCM client was never configured.
var ErrNotConfigured = errors.New("fcm: not configured")
