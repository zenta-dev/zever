package lock

import (
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

const (
	// DefaultTTL is the lease lifetime used when neither the caller nor the
	// configuration supplies one.
	DefaultTTL = 30 * time.Second
	// DefaultRetryInterval is how long Acquire waits between attempts.
	DefaultRetryInterval = 50 * time.Millisecond
)

// Options holds typed configuration for the lock battery.
// Fields are a union of all adapter options; each adapter uses only what it needs.
type Options struct {
	// ConnectOptions holds the shared Redis connection settings.
	redisopt.ConnectOptions
	// URL is the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`
	// Prefix scopes lock keys to one namespace.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// TTL is the default lease lifetime.
	TTL time.Duration `json:"ttl" toml:"ttl" yaml:"ttl"`
	// RetryInterval is how long Acquire waits between attempts.
	RetryInterval time.Duration `json:"retry_interval" toml:"retry_interval" yaml:"retry_interval"`
	// MaxEntries bounds the number of stored leases before LRU eviction.
	// Past the bound the least-recently-used live lease is evicted and its
	// holder observes lock.ErrNotHeld on Extend/Unlock. Non-positive means
	// the memory adapter default (1000). Callers needing effectively
	// unbounded storage set a large value knowingly.
	MaxEntries int `json:"max_entries" toml:"max_entries" yaml:"max_entries"`
}

// Validate checks options for consistency, joining all violations.
// There are currently no checks that hold universally across all adapters,
// so this is a no-op; each adapter's factory validates its own fields.
func (o Options) Validate() error {
	return nil
}
