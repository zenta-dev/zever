package auth

import (
	"time"

	"github.com/zenta-dev/zever/core/auth/revocation"
	"github.com/zenta-dev/zever/core/session"
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
	Secret string `json:"secret" toml:"secret" yaml:"secret"`
	// Issuer is the token issuer. Required (adapter-validated).
	Issuer string `json:"issuer" toml:"issuer" yaml:"issuer"`
	// Audience is the intended token audience.
	Audience string `json:"audience" toml:"audience" yaml:"audience"`
	// MaxTTL caps token lifetimes. Negative fails Validate; zero means no cap.
	MaxTTL time.Duration `json:"max_ttl" toml:"max_ttl" yaml:"max_ttl"`
	// RevocationStore tracks revoked token jtis. Nil means the adapter
	// builds an in-process memory store with its own defaults.
	RevocationStore revocation.Store `json:"-" toml:"-" yaml:"-"`
}

// SessionOptions configures the session-backed adapter.
type SessionOptions struct {
	// Store is the session backend. Required: nil fails New with
	// *InvalidOptionsError since the adapter no longer builds a default.
	Store session.Store `json:"-" toml:"-" yaml:"-"`
}

// OIDCOptions configures the OpenID Connect adapter.
type OIDCOptions struct {
	// Issuer is the OIDC issuer URL. Required (adapter-validated).
	Issuer string `json:"issuer" toml:"issuer" yaml:"issuer"`
	// ClientID is the relying-party client ID. Required (adapter-validated).
	ClientID string `json:"client_id" toml:"client_id" yaml:"client_id"`
	// Timeout is the operation timeout. Zero means the default; negative fails.
	Timeout time.Duration `json:"timeout" toml:"timeout" yaml:"timeout"`
}

// Options configures auth backend construction.
type Options struct {
	// JWT carries the JWT adapter settings.
	JWT JWTOptions `json:"jwt" toml:"jwt" yaml:"jwt"`
	// Session carries the session adapter settings.
	Session SessionOptions `json:"session" toml:"session" yaml:"session"`
	// OIDC carries the OIDC adapter settings.
	OIDC OIDCOptions `json:"oidc" toml:"oidc" yaml:"oidc"`
}

// Validate checks options for consistency, joining all violations.
// Zero values are valid and mean "apply default".
// Per-adapter required fields (secret length, issuer, client ID)
// are validated by the adapters themselves.
func (o Options) Validate() error {
	if o.JWT.MaxTTL < 0 {
		return &InvalidOptionsError{Reason: "jwt_max_ttl must be >= 0"}
	}
	if o.OIDC.Timeout < 0 {
		return &InvalidOptionsError{Reason: "oidc_timeout must be >= 0"}
	}
	return nil
}
