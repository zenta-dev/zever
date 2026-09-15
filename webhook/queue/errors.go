package queue

import "errors"

// ErrMissingQueueAdapter is returned when Options names no queue backend.
var ErrMissingQueueAdapter = errors.New("queue: queue adapter is required")

// ErrVisibilityTimeout is returned when the queue visibility timeout is not positive.
var ErrVisibilityTimeout = errors.New("queue: visibility timeout must be greater than timeout")
