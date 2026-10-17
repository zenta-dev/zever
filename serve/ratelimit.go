package serve

import (
	"fmt"
	"net/http"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/middleware"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// resolveRateLimit returns the limiter and key function when the resolved
// config carries a real rate, or nils when rate limiting is off (Rate <=
// 0). A configured-but-unresolvable limiter fails the boot: silently
// serving unprotected is the fail-open shape.
func resolveRateLimit(c *container.Container, cfg *config.Config) (ratelimit.Limiter, func(*http.Request) string, error) {
	if cfg.RateLimit.Options.Rate <= 0 {
		return nil, nil, nil
	}

	limiter, err := c.RateLimit()
	if err != nil {
		return nil, nil, fmt.Errorf("serve: resolve ratelimit: %w", err)
	}

	trustedProxies, err := middleware.ParseTrustedProxies(cfg.RateLimit.Options.TrustedProxies)
	if err != nil {
		return nil, nil, fmt.Errorf("serve: parse trusted proxies (RATELIMIT_TRUSTED_PROXIES): %w", err)
	}

	// Without trusted proxies the key stays the peer address: honoring
	// X-Forwarded-For from an untrusted peer would let any client pick
	// its own rate-limit bucket.
	keyFunc := middleware.RemoteAddrKey
	if len(trustedProxies) > 0 {
		keyFunc = middleware.ClientIPKey(trustedProxies)
	}

	return limiter, keyFunc, nil
}
