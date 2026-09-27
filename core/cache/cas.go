package cache

import (
	"context"
	"time"
)

// CompareAndSwapCache is an optional capability a Cache may implement for
// atomic holder-checked delete and TTL renewal. It powers lock-style leases
// without widening the base Cache interface, so third-party adapters keep
// working without changes. Expected holds the exact value that must still be
// present for the operation to succeed. Tokens use []byte for strict typing.
type CompareAndSwapCache interface {
	// CompareAndDelete removes key only when its live value equals expected.
	// It reports deleted=false with nil error when the key is missing,
	// expired, or holds another value, so callers never steal a successor.
	CompareAndDelete(ctx context.Context, key string, expected []byte) (bool, error)
	// CompareAndExtend renews the TTL on key only when its live value equals
	// expected. A non-positive ttl clears the expiry. It reports
	// extended=false with nil error when the key is missing, expired, or
	// holds another value.
	CompareAndExtend(ctx context.Context, key string, expected []byte, ttl time.Duration) (bool, error)
}
