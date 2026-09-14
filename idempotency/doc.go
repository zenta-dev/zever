// Package idempotency provides idempotent execution primitives.
//
// A Store reserves a caller-supplied key before execution (Begin) and
// records the result after execution (Complete), so retries with the same
// key replay the stored result instead of re-executing. All errors are
// fail-closed: on any error the caller must not assume execution happened.
package idempotency
