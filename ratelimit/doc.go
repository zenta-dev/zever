// Package ratelimit defines a token-bucket rate limiter facade with swappable adapters.
//
// Allow reports an error for infrastructure failures only. Callers decide
// fail-open behavior on error. A denial is not an error: it is returned as
// (Decision{Allowed: false, RetryAfter: ..., Remaining: ...}, nil).
// HTTP middleware maps a denial to 429 with a Retry-After header.
package ratelimit
