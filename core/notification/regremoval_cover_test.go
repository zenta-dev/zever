package notification

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise the adapter
// errors via duplicate/unknown/invalid paths, so direct unit tests now pin
// the message constructors, sentinel unwrapping, and As round-trips.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSubs []string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "fcm"}, ErrDuplicateAdapter, []string{"fcm"}, &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "fcm"}, ErrUnknownAdapter, []string{"fcm"}, &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, []string{"bogus"}, &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad key"}, ErrInvalidOptions, []string{"bad key"}, &InvalidOptionsError{}},
		{"notification", InvalidNotificationError{Reason: "empty body"}, ErrInvalidNotification, []string{"empty body"}, &InvalidNotificationError{}},
		{"target", InvalidTargetError{Reason: "bad token"}, ErrInvalidTarget, []string{"bad token"}, &InvalidTargetError{}},
		{"channel", InvalidChannelError{Channel: "bogus"}, ErrInvalidChannel, []string{"bogus"}, &InvalidChannelError{}},
		{"unsupported", ChannelNotSupportedError{Channel: ChannelSMS}, ErrChannelNotSupported, []string{"sms"}, &ChannelNotSupportedError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "notification: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "notification: ")
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("Error() = %q, want substring %q", msg, sub)
				}
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}
