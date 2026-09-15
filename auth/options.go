package auth

import (
	"time"

	"github.com/zenta-dev/zever/session"
)

const (
	// DefaultTimeout is the default auth operation timeout applied by adapters.
	DefaultTimeout = 30 * time.Second
	// MinSecretLen is the minimum accepted secret length for adapters
	// that use shared secrets. Adapters enforce this.
	MinSecretLen = 32
)

// JWTOptions configures the JWT adapter.
type JWTOptions struct {
	// Secret is the HMAC signing secret. Must be at least MinSecretLen bytes (adapter-validated).
	Secret string
	// Issuer is the token issuer. Required (adapter-validated).
	Issuer string
	// Audience is the intended token audience.
	Audience string
	// MaxTTL caps token lifetimes. Negative fails Validate; zero means no cap.
	MaxTTL time.Duration
}

// SessionOptions configures the session-backed adapter.
type SessionOptions struct {
	// Store is the session backend. Nil means the adapter builds a
	// memory store with session defaults.
	Store session.Store
}

// OIDCOptions configures the OpenID Connect adapter.
type OIDCOptions struct {
	// Issuer is the OIDC issuer URL. Required (adapter-validated).
	Issuer string
	// ClientID is the relying-party client ID. Required (adapter-validated).
	ClientID string
	// Timeout is the operation timeout. Zero means the default; negative fails.
	Timeout time.Duration
}

// Options configures auth backend construction.
type Options struct {
	// JWT carries the JWT adapter settings.
	JWT JWTOptions
	// Session carries the session adapter settings.
	Session SessionOptions
	// OIDC carries the OIDC adapter settings.
	OIDC OIDCOptions
}

// Validate checks options for consistency.
// Zero values are valid and mean "apply default".
// Per-adapter required fields (secret length, issuer, client ID)
// are validated by the adapters themselves.
func (o Options) Validate() error {
	if o.JWT.MaxTTL < 0 {
		return &InvalidOptionsError{Reason: "jwt maxttl must be >= 0"}
	}
	if o.OIDC.Timeout < 0 {
		return &InvalidOptionsError{Reason: "oidc timeout must be >= 0"}
	}
	return nil
}
