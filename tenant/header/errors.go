package header

import "errors"

// ErrNilMeta is returned when resolution metadata is nil.
var ErrNilMeta = errors.New("header: meta is nil")

// ErrInvalidHost is returned when a Host value cannot be parsed.
var ErrInvalidHost = errors.New("header: invalid host")

// ErrHostTooLong is returned when a Host value exceeds the DNS length limit.
var ErrHostTooLong = errors.New("header: host too long")

// ErrInvalidPattern is returned when a subdomain pattern is rejected.
var ErrInvalidPattern = errors.New("header: invalid subdomain pattern")
