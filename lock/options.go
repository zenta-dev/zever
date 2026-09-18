package lock

import (
	"fmt"
	"time"

	"github.com/zenta-dev/zever/internal/opts"
	zredis "github.com/zenta-dev/zever/internal/redis"
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
	zredis.ConnectOptions
	// URL is the Redis connection URL.
	// When set it takes precedence over Addr.
	URL string `json:"url" toml:"url" yaml:"url"`
	// Prefix scopes lock keys to one namespace.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// TTL is the default lease lifetime.
	TTL time.Duration `json:"ttl" toml:"ttl" yaml:"ttl"`
	// RetryInterval is how long Acquire waits between attempts.
	RetryInterval time.Duration `json:"retry_interval" toml:"retry_interval" yaml:"retry_interval"`
}

var lockOptionKeys = map[string]struct{}{
	"url": {}, "addr": {}, "password": {}, "db": {}, "tls": {}, "require_tls": {}, "prefix": {}, "ttl": {}, "retry_interval": {},
}

// ParseOptions extracts a typed Options from the raw option map.
// Unknown keys and wrong-typed values return an error.
func ParseOptions(m map[string]any) (Options, error) {
	if m == nil {
		m = map[string]any{}
	}

	for k := range m {
		if _, ok := lockOptionKeys[k]; !ok {
			return Options{}, fmt.Errorf("lock: unknown option %q", k)
		}
	}

	var o Options

	o.Addr = "localhost:6379"
	o.Prefix = "lock:"
	o.TTL = DefaultTTL
	o.RetryInterval = DefaultRetryInterval

	for k, v := range m {
		var err error

		switch k {
		case "url":
			o.URL, err = opts.StrictString("lock", k, v)
		case "addr":
			o.Addr, err = opts.StrictString("lock", k, v)
		case "password":
			o.Password, err = opts.StrictString("lock", k, v)
		case "db":
			o.DB, err = opts.StrictInt("lock", k, v)
		case "tls":
			o.TLS, err = opts.StrictBool("lock", k, v)
		case "require_tls":
			o.RequireTLS, err = opts.StrictBool("lock", k, v)
		case "prefix":
			o.Prefix, err = opts.StrictString("lock", k, v)
		case "ttl":
			o.TTL, err = opts.StrictDuration("lock", k, v)
		case "retry_interval":
			o.RetryInterval, err = opts.StrictDuration("lock", k, v)
		}

		if err != nil {
			return Options{}, err
		}
	}

	return o, nil
}

// Validate performs battery-level checks that hold across every adapter.
// There are currently no checks that hold universally across all adapters,
// so this is a no-op; each adapter's factory validates its own fields.
func (o Options) Validate() error {
	return nil
}
