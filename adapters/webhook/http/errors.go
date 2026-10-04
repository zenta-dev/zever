package http

import "errors"

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
