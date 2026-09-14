// Package memory provides an in-process token-bucket rate limiter.
//
// Each key gets its own bucket holding up to burst tokens, refilled at
// rate tokens per second using monotonic time. Buckets idle longer than
// IdleTTL are reclaimed by a background sweeper and opportunistically
// on lookup.
package memory
