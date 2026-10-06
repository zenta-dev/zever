package redis

import "errors"

// ErrNoSharedState is returned when the distributed breaker's shared state is
// missing or unreadable from Redis. It wraps gobreaker.ErrNoSharedState.
var ErrNoSharedState = errors.New("redis: no shared breaker state")
