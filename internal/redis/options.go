package redis

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Options configures the Redis connection and pooling behavior.
type Options struct {
	// URL is reserved and currently ignored.
	URL string
	// ConnectOptions holds the shared connection settings.
	ConnectOptions

	// PoolSize is the base number of socket connections. <= 0 uses go-redis's
	// own default (10 * GOMAXPROCS).
	PoolSize int
	// MinIdleConns is the minimum number of idle connections to keep
	// pre-warmed. <= 0 defaults to a small warm pool of 2, to avoid
	// cold-start dial latency on the first requests.
	MinIdleConns int
	// PoolTimeout is how long a caller waits for a connection when the pool
	// is exhausted. <= 0 uses go-redis's own default.
	PoolTimeout time.Duration
	// ConnMaxIdleTime is the maximum time a connection may sit idle before
	// it's eligible for closing. <= 0 uses go-redis's own default.
	ConnMaxIdleTime time.Duration
	// ConnMaxLifetime is the maximum time a connection may be reused before
	// it's eligible for closing. <= 0 uses go-redis's own default (unbounded).
	ConnMaxLifetime time.Duration
}

func (o Options) toRedisOptions() (*goredis.Options, error) {
	addr := strings.TrimSpace(o.Addr)
	if addr == "" {
		addr = "localhost:6379"
	}

	password := strings.TrimSpace(o.Password)

	minIdleConns := o.MinIdleConns
	if minIdleConns <= 0 {
		minIdleConns = 2
	}

	// Support redis:// and rediss:// URLs.
	if strings.HasPrefix(strings.ToLower(addr), "redis://") ||
		strings.HasPrefix(strings.ToLower(addr), "rediss://") {
		u, err := url.Parse(addr)
		if err != nil {
			return nil, &InvalidAddressError{Addr: addr, Err: fmt.Errorf("%w: %w", ErrParseAddress, err)}
		}

		if u.Host == "" {
			return nil, &InvalidAddressError{Addr: addr, Err: errors.New("missing host")}
		}

		redisOpt := &goredis.Options{
			Addr:            u.Host,
			Password:        password,
			DB:              o.DB,
			PoolSize:        o.PoolSize,
			MinIdleConns:    minIdleConns,
			PoolTimeout:     o.PoolTimeout,
			ConnMaxIdleTime: o.ConnMaxIdleTime,
			ConnMaxLifetime: o.ConnMaxLifetime,
		}

		// Password from the URL is used only when Options.Password
		// wasn't explicitly supplied.
		if redisOpt.Password == "" {
			if p, ok := u.User.Password(); ok {
				redisOpt.Password = p
			}
		}

		// rediss:// implies TLS, while Options.TLS can explicitly
		// enable TLS for a normal host:port address.
		tlsEnabled := o.TLS || strings.EqualFold(u.Scheme, "rediss")
		if tlsEnabled {
			redisOpt.TLSConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		} else if o.RequireTLS {
			return nil, &PlaintextRejectedError{Addr: addr}
		}

		return redisOpt, nil
	}

	opt := &goredis.Options{
		Addr:            addr,
		Password:        password,
		DB:              o.DB,
		PoolSize:        o.PoolSize,
		MinIdleConns:    minIdleConns,
		PoolTimeout:     o.PoolTimeout,
		ConnMaxIdleTime: o.ConnMaxIdleTime,
		ConnMaxLifetime: o.ConnMaxLifetime,
	}

	if o.TLS {
		opt.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	} else if o.RequireTLS {
		return nil, &PlaintextRejectedError{Addr: addr}
	}

	return opt, nil
}
