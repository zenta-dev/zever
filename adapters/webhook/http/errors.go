package http

import (
	"errors"
	"fmt"
)

// ErrMissingEvent is returned when the event name is empty.
var ErrMissingEvent = errors.New("http: event is empty")

// ErrMissingTarget is returned when the target URL is empty.
var ErrMissingTarget = errors.New("http: target is empty")

// ErrMissingSecret is returned when Register is called without a target
// secret. Every delivery must carry X-Hub-Signature-256 so the receiver
// can verify it, so an empty secret is rejected at registration.
var ErrMissingSecret = errors.New("http: target secret is empty")

// ErrDeliveryFailed is returned when every delivery attempt is exhausted.
var ErrDeliveryFailed = errors.New("http: delivery failed")

// ErrUnexpectedStatus is returned when the target replies with a non-2xx status.
var ErrUnexpectedStatus = errors.New("http: unexpected status")

// deliveryError reports a delivery that failed on every attempt. It names
// the event and the target, so a developer can tell which registration
// broke without reading the adapter's source.
//
// It renders on one line: a joined error (errors.Join) renders with a
// newline, and every single-line consumer -- the `zever doctor` table row,
// a log line -- would then lose the cause.
type deliveryError struct {
	// Event is the delivered event name.
	Event string
	// Target is the delivery target URL.
	Target string
	// Attempts is the number of attempts made.
	Attempts int
	// Err is the last attempt's failure.
	Err error
}

// Error returns a human-readable delivery failure message.
func (e *deliveryError) Error() string {
	return fmt.Sprintf("http: deliver event %q to %q failed after %d attempts: %v",
		e.Event, redactTarget(e.Target), e.Attempts, e.Err)
}

// Unwrap returns ErrDeliveryFailed joined with the last attempt's failure,
// so errors.Is matches both.
func (e *deliveryError) Unwrap() error {
	return errors.Join(ErrDeliveryFailed, e.Err)
}
