package redis

import "errors"

// ErrUnexpectedResult is returned when the allow script returns an unexpected shape.
var ErrUnexpectedResult = errors.New("redis: unexpected script result")
