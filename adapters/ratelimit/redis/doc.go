// Package redis provides a Redis-backed ratelimit.Limiter.
//
// Design: the token bucket lives in a Redis hash ({tokens, last}) updated
// by an atomic Lua script. Client time (now) is passed as ARGV instead of
// calling Redis TIME inside the write script: TIME is non-deterministic and
// forbidden after writes on Redis 7.2+ ("Write commands not allowed after
// non deterministic commands"), breaking replication. Passing now keeps the
// script deterministic, replica-safe, and test-injectable.
//
// Cost stays float (Lua tonumber). The script returns ints only (Lua floats
// truncate on return): {allowed, remaining floor, retry_ms}.
//
// Allow returns errors for infrastructure failures only; the fail-open
// decision is left to the caller. A denial is
// (Decision{Allowed: false, RetryAfter: ..., Remaining: ...}, nil).
// Close is idempotent and never closes the shared internal/redis pool.
package redis
