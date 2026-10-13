package auth

import (
	"errors"
	"time"

	"github.com/zenta-dev/zever/core/auth/revocation"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/shared/endpoint"
)

const (
	// DefaultTimeout is the default auth operation timeout applied by adapters.
	DefaultTimeout = 30 * time.Second
	// MinSecretLen is the minimum accepted secret length for adapters
	// that use shared secrets. Adapters enforce this.
	MinSecretLen = 32
)

// JWTKey is one rotatable HMAC signing key in JWTOptions.Keys.
type JWTKey struct {
	// KID is the key ID sent as the token "kid" header. Required,
	// unique within Keys.
	KID string `json:"kid" toml:"kid" yaml:"kid"`
	// Secret is the HMAC signing secret. Must be at least MinSecretLen
	// bytes (adapter-validated).
	Secret string `json:"secret" toml:"secret" yaml:"secret" zever:"secret"`
}

// JWTOptions configures the JWT adapter.
type JWTOptions struct {
	// Secret is the HMAC signing secret. Must be at least MinSecretLen bytes (adapter-validated).
	Secret string `json:"secret" toml:"secret" yaml:"secret" zever:"secret"`
	// Keys holds rotatable HMAC signing keys by key ID. Tokens are
	// signed with the ActiveKID key (or Keys[0] when ActiveKID is empty)
	// and carry its ID in the "kid" header; verification selects the key
	// by "kid" and falls back to Secret for kid-less (legacy) tokens.
	// Old keys must stay listed until every token signed with them has
	// expired, otherwise those tokens fail closed as invalid.
	Keys []JWTKey `json:"keys" toml:"keys" yaml:"keys"`
	// ActiveKID selects the signing key within Keys. Empty defaults to
	// Keys[0]. Must match a Keys entry when set; setting it with no Keys
	// fails Validate.
	ActiveKID string `json:"active_kid" toml:"active_kid" yaml:"active_kid"`
	// Issuer is the token issuer. Required (adapter-validated).
	Issuer string `json:"issuer" toml:"issuer" yaml:"issuer"`
	// Audience is the intended token audience.
	Audience string `json:"audience" toml:"audience" yaml:"audience"`
	// MaxTTL caps token lifetimes. Negative fails Validate; zero means no cap.
	MaxTTL time.Duration `json:"max_ttl" toml:"max_ttl" yaml:"max_ttl"`
	// Leeway tolerates clock skew when validating exp/nbf. Zero (default)
	// means strict validation, preserving current behavior; positive values
	// accept tokens up to Leeway past expiry. Negative fails Validate.
	Leeway time.Duration `json:"leeway" toml:"leeway" yaml:"leeway"`
	// RevocationStore tracks revoked token jtis. Nil means the adapter
	// builds an in-process memory store with its own defaults. The memory
	// store is per-process: revocations are lost on restart and invisible
	// to other instances, so multi-instance deployments must inject a
	// shared store (core/auth/revocation/redis, or a custom revocation.Store).
	RevocationStore revocation.Store `json:"-" toml:"-" yaml:"-"`
}

// validateJWTKeys checks the rotatable Keys/ActiveKID pair for structural
// consistency. Secret length and presence are adapter-validated (the legacy
// Secret-only path must keep working), but key entries are new surface and
// fail fast here: empty kids, duplicate kids, and an ActiveKID with no
// matching entry are all rejected.
func validateJWTKeys(j JWTOptions) error {
	if j.ActiveKID != "" && len(j.Keys) == 0 {
		return InvalidOptionsError{Reason: "jwt_active_kid set with no jwt_keys"}
	}
	seen := make(map[string]struct{}, len(j.Keys))
	for _, k := range j.Keys {
		if k.KID == "" {
			return InvalidOptionsError{Reason: "jwt_keys entry must have a kid"}
		}
		if _, dup := seen[k.KID]; dup {
			return InvalidOptionsError{Reason: "jwt_keys has duplicate kid"}
		}
		seen[k.KID] = struct{}{}
		if len([]byte(k.Secret)) < MinSecretLen {
			return InvalidOptionsError{Reason: "jwt_keys entry secret must be long enough"}
		}
	}
	if j.ActiveKID != "" {
		if _, ok := seen[j.ActiveKID]; !ok {
			return InvalidOptionsError{Reason: "jwt_active_kid matches no jwt_keys entry"}
		}
	}
	return nil
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
	// AllowInsecure permits an http:// issuer (and a private-address issuer)
	// for testing. Default false (https and public addresses only).
	AllowInsecure bool `json:"allow_insecure" toml:"allow_insecure" yaml:"allow_insecure"`
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
		return InvalidOptionsError{Reason: "jwt_max_ttl must be >= 0"}
	}
	if o.JWT.Leeway < 0 {
		return InvalidOptionsError{Reason: "jwt_leeway must be >= 0"}
	}
	if err := validateJWTKeys(o.JWT); err != nil {
		return err
	}
	if o.OIDC.Timeout < 0 {
		return InvalidOptionsError{Reason: "oidc_timeout must be >= 0"}
	}
	if o.OIDC.Issuer != "" {
		if _, err := endpoint.ValidateURL(o.OIDC.Issuer, endpoint.WithAllowInsecure(o.OIDC.AllowInsecure)); err != nil {
			if errors.Is(err, endpoint.ErrInsecureScheme) || errors.Is(err, endpoint.ErrUnsupportedScheme) {
				return InvalidOptionsError{Reason: "oidc issuer must use https or set allow_insecure"}
			}
			return InvalidOptionsError{Reason: "oidc issuer must be a valid url with scheme and host"}
		}
	}
	return nil
}
