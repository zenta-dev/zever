package ratelimit

import (
	"fmt"
	"math"
	"net/netip"
	"strings"
	"time"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

const (
	// DefaultIdleTTL is the default idle entry TTL applied by adapters.
	DefaultIdleTTL = 10 * time.Minute
	// DefaultSweepInterval is the default idle-entry sweep interval applied by adapters for reaping idle entries.
	DefaultSweepInterval = time.Minute
	// DefaultMaxEntries bounds the memory adapter's bucket table before LRU eviction.
	DefaultMaxEntries = 1000
	// MaxKeyLen is the maximum allowed rate-limit key length.
	MaxKeyLen = 256
)

// RedisOptions holds connection settings for the Redis adapter.
type RedisOptions struct {
	// Options holds the shared Redis connection AND pooling settings
	// (PoolSize, MinIdleConns, PoolTimeout, MaxConnIdleTime,
	// MaxConnLifetime) -- previously only ConnectOptions was embedded here,
	// silently dropping every pooling knob to go-redis's defaults with no
	// way for a caller to tune them for a hot ratelimit store.
	redisopt.Options
	// Prefix is the key prefix for Redis ratelimit data.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
}

// Options configures ratelimit behavior and adapter-specific settings.
type Options struct {
	// Rate is the token refill rate per second. Required, must be > 0 and finite.
	Rate float64 `json:"rate" toml:"rate" yaml:"rate"`
	// Burst is the maximum bucket size. Required, must be > 0.
	Burst int `json:"burst" toml:"burst" yaml:"burst"`
	// IdleTTL is the idle entry TTL. Zero means the adapter default; negative fails.
	IdleTTL time.Duration `json:"idle_ttl" toml:"idle_ttl" yaml:"idle_ttl"`
	// SweepInterval is the idle-entry sweep interval. Zero means the adapter default; negative fails.
	SweepInterval time.Duration `json:"sweep_interval" toml:"sweep_interval" yaml:"sweep_interval"`
	// MaxEntries bounds the number of stored buckets in the memory adapter
	// before least-recently-used eviction. Non-positive means DefaultMaxEntries.
	MaxEntries int `json:"max_entries" toml:"max_entries" yaml:"max_entries"`
	// Redis holds Redis-specific connection configuration.
	Redis RedisOptions `json:"redis" toml:"redis" yaml:"redis"`
	// TrustedProxies lists the CIDRs (for example "10.0.0.0/8") or bare
	// IPs (for example "10.0.0.1", meaning /32 or /128) of proxies whose
	// X-Forwarded-For entries may be trusted for client-IP derivation.
	// Empty means no proxy is trusted and X-Forwarded-For is ignored.
	TrustedProxies []string `json:"trusted_proxies" toml:"trusted_proxies" yaml:"trusted_proxies"`
}

// ParseTrustedProxies parses proxies into CIDR prefixes, in order. Each
// entry must be a CIDR (for example "10.0.0.0/8") or a bare IP (for
// example "10.0.0.1", treated as a single-host /32 or /128 prefix).
// Surrounding whitespace is ignored. An empty input returns nil with no
// error. Failures return an InvalidOptionsError naming the bad entry, so
// generated servers can fail fast at boot with an actionable message.
func ParseTrustedProxies(proxies []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(proxies))
	for i, raw := range proxies {
		s := strings.TrimSpace(raw)
		if p, err := netip.ParsePrefix(s); err == nil {
			prefixes = append(prefixes, p.Masked())

			continue
		}
		if addr, err := netip.ParseAddr(s); err == nil {
			addr = addr.Unmap()
			bits := 128
			if addr.Is4() {
				bits = 32
			}
			prefixes = append(prefixes, netip.PrefixFrom(addr, bits))

			continue
		}

		return nil, InvalidOptionsError{Reason: fmt.Sprintf("trusted_proxies entry %d is not a valid CIDR or IP: %q", i, s)}
	}

	return prefixes, nil
}

// Validate checks options for consistency, joining all violations.
func (o Options) Validate() error {
	if math.IsNaN(o.Rate) || math.IsInf(o.Rate, 0) || o.Rate <= 0 {
		return InvalidOptionsError{Reason: "rate must be > 0 and finite"}
	}

	if o.Burst <= 0 {
		return InvalidOptionsError{Reason: "burst must be > 0"}
	}

	if o.IdleTTL < 0 {
		return InvalidOptionsError{Reason: "idle_ttl must be >= 0"}
	}

	if o.SweepInterval < 0 {
		return InvalidOptionsError{Reason: "sweep_interval must be >= 0"}
	}

	if err := redisopt.ValidateAddr(o.Redis.Addr); err != nil {
		return InvalidOptionsError{Reason: err.Error()}
	}

	if err := redisopt.ValidatePrefix(o.Redis.Prefix); err != nil {
		return InvalidOptionsError{Reason: err.Error()}
	}

	if _, err := ParseTrustedProxies(o.TrustedProxies); err != nil {
		return err
	}

	return nil
}

// idleTTL returns IdleTTL or DefaultIdleTTL when unset.
func (o Options) idleTTL() time.Duration {
	if o.IdleTTL <= 0 {
		return DefaultIdleTTL
	}

	return o.IdleTTL
}

// sweepInterval returns SweepInterval or DefaultSweepInterval when unset.
func (o Options) sweepInterval() time.Duration {
	if o.SweepInterval <= 0 {
		return DefaultSweepInterval
	}

	return o.SweepInterval
}
