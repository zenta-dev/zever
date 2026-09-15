package http

import "errors"

// ErrMissingEvent is returned when the event name is empty.
var ErrMissingEvent = errors.New("http: event is empty")

// ErrMissingTarget is returned when the target URL is empty.
var ErrMissingTarget = errors.New("http: target is empty")
