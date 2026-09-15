package oidc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/zenta-dev/zever/auth"
)

// adapter verifies OIDC ID tokens against a discovered provider.
type adapter struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	timeout  time.Duration
}

// var _ auth.Auth pins the interface at compile time.
var _ auth.Auth = (*adapter)(nil)

// New builds an OIDC adapter. It validates options, then runs OIDC
// discovery against opts.OIDC.Issuer with a bounded timeout.
func New(opts auth.Options) (auth.Auth, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("oidc: %w", err)
	}

	issuer := opts.OIDC.Issuer
	if issuer == "" {
		return nil, &auth.InvalidOptionsError{Reason: "oidc issuer must be non-empty"}
	}
	if opts.OIDC.ClientID == "" {
		return nil, &auth.InvalidOptionsError{Reason: "oidc client id must be non-empty"}
	}

	timeout := opts.OIDC.Timeout
	if timeout <= 0 {
		timeout = auth.DefaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discovery: %w", err)
	}

	return &adapter{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: opts.OIDC.ClientID}),
		timeout:  timeout,
	}, nil
}

// Issue always fails: the IdP mints tokens, never this adapter.
func (a *adapter) Issue(_ context.Context, _ string, _ map[string]any, _ time.Duration) (auth.Token, error) {
	return auth.Token{}, fmt.Errorf("oidc: issue: %w", auth.ErrNotSupported)
}

// profileClaims carries the optional user-profile claims extracted from a
// verified ID token. All three keys are always present in Claims.Custom,
// including zero values when the IdP omits them.
type profileClaims struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
}

// Verify authenticates rawToken and returns its claims. The token value is
// a secret and is never echoed in errors.
func (a *adapter) Verify(ctx context.Context, token string) (auth.Claims, error) {
	if token == "" {
		return auth.Claims{}, auth.ErrInvalidToken
	}

	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	idToken, err := a.verifier.Verify(ctx, token)
	if err != nil {
		var expired *oidc.TokenExpiredError
		if errors.As(err, &expired) {
			return auth.Claims{}, fmt.Errorf("oidc: verify: %w: %w", auth.ErrTokenExpired, err)
		}
		return auth.Claims{}, fmt.Errorf("oidc: verify: %w: %w", auth.ErrInvalidToken, err)
	}

	// Payload of a verified token is valid JSON; unmarshal failure here
	// means something is deeply wrong, so reject the token.
	var profile profileClaims
	if err := idToken.Claims(&profile); err != nil {
		return auth.Claims{}, fmt.Errorf("oidc: verify: claims: %w: %w", auth.ErrInvalidToken, err)
	}

	return auth.Claims{
		Subject: idToken.Subject,
		Custom: map[string]any{
			"email":          profile.Email,
			"email_verified": profile.EmailVerified,
			"name":           profile.Name,
		},
		ExpiresAt: idToken.Expiry,
	}, nil
}

// Revoke always fails: the IdP owns the token lifecycle, never this adapter.
func (a *adapter) Revoke(_ context.Context, _ string) error {
	return fmt.Errorf("oidc: revoke: %w", auth.ErrNotSupported)
}

// Close releases adapter resources. The provider holds no closeable
// resources, so this is always nil.
func (a *adapter) Close() error {
	return nil
}
