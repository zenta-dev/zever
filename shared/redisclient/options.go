package redisclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/shared/redisopt"
)

// Options configures the Redis connection and pooling behavior.
//
// It is an alias of redisopt.Options, so facade packages embed the light
// type while the nested */redis adapter modules pass it here unchanged.
type Options = redisopt.Options

func toRedisOptions(o Options) (*goredis.Options, error) {
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
			ConnMaxIdleTime: o.MaxConnIdleTime,
			ConnMaxLifetime: o.MaxConnLifetime,
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
		ConnMaxIdleTime: o.MaxConnIdleTime,
		ConnMaxLifetime: o.MaxConnLifetime,
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
