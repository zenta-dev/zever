package queue

import "errors"

// ErrMissingQueueAdapter is returned when Options names no queue backend.
var ErrMissingQueueAdapter = errors.New("queue: queue adapter is required")

// ErrVisibilityTimeout is returned when the queue visibility timeout is not positive.
var ErrVisibilityTimeout = errors.New("queue: visibility timeout must be greater than timeout")

// ErrSignatureMismatch is returned when a webhook signature's HMAC does not
// match its payload: the payload was tampered with, the secret is wrong, or
// the signature header is malformed.
var ErrSignatureMismatch = errors.New("queue: webhook signature mismatch")

// ErrSignatureExpired is returned when a webhook signature's embedded
// timestamp falls outside the configured replay tolerance window, even
// though its HMAC matches. This signals an expired or replayed delivery
// rather than a tampered one.
var ErrSignatureExpired = errors.New("queue: webhook signature timestamp outside tolerance window")
