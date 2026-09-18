// Package jwt provides an HS256 JWT authentication adapter.
//
// Custom claims are merged at the top level of the token alongside the
// registered claims (sub, iss, aud, exp, iat, jti). Custom keys colliding
// with a registered claim name are rejected at Issue time.
//
// Security notes:
//   - HS256 only: the keyfunc rejects any token whose alg header is not
//     exactly "HS256", and the parser is additionally constrained with
//     jwt.WithValidMethods. This blocks none/RS256-confusion attacks.
//   - issuer/audience: when configured they are enforced via
//     jwt.WithIssuer/WithAudience. When empty, tokens carrying an
//     issuer/audience are rejected to avoid cross-tenant bypass when the
//     same HMAC secret is shared across tenants.
package jwt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/auth/jwt/revocation"
	revocationmemory "github.com/zenta-dev/zever/auth/jwt/revocation/memory"
)

// randReader is the CSPRNG source for JTIs, swappable in tests to
// simulate host failure. Production always uses crypto/rand.
var randReader = rand.Reader

// registeredKeys are claim names owned by the adapter and rejected as
// custom claim keys at Issue time.
var registeredKeys = map[string]struct{}{
	"iss": {}, "sub": {}, "aud": {}, "exp": {}, "nbf": {}, "iat": {}, "jti": {},
}

var _ auth.Auth = (*adapter)(nil)

type adapter struct {
	secret   []byte
	issuer   string
	audience string
	maxTTL   time.Duration

	revocation revocation.Store
}

// New builds an HS256 JWT Auth from opts.
//
// opts is validated with auth.Options.Validate first (wrapped as "jwt: %w").
// The JWT secret is required and must be at least auth.MinSecretLen bytes,
// else a *auth.InvalidOptionsError is returned.
func New(opts auth.Options) (auth.Auth, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}
	secret := opts.JWT.Secret
	if secret == "" {
		return nil, &auth.InvalidOptionsError{Reason: "jwt secret is required"}
	}
	if len([]byte(secret)) < auth.MinSecretLen {
		return nil, &auth.InvalidOptionsError{
			Reason: fmt.Sprintf("jwt secret must be at least %d bytes", auth.MinSecretLen),
		}
	}

	store := opts.JWT.RevocationStore
	if store == nil {
		// memory.New with zero Options performs no I/O and cannot fail:
		// the store allocates no external resources.
		store, _ = revocationmemory.New(revocationmemory.Options{})
	}

	a := &adapter{
		secret:     append([]byte(nil), []byte(secret)...),
		issuer:     opts.JWT.Issuer,
		audience:   opts.JWT.Audience,
		maxTTL:     opts.JWT.MaxTTL,
		revocation: store,
	}
	return a, nil
}

// keyfunc enforces HS256 with an exact, case-sensitive alg comparison.
func (a *adapter) keyfunc(t *jwtv5.Token) (any, error) {
	if t.Method.Alg() != "HS256" {
		return nil, fmt.Errorf("jwt: unexpected signing method %q", t.Method.Alg())
	}
	return a.secret, nil
}

func (a *adapter) parserOptions() []jwtv5.ParserOption {
	options := []jwtv5.ParserOption{
		jwtv5.WithExpirationRequired(),
		jwtv5.WithValidMethods([]string{"HS256"}),
	}
	if a.issuer != "" {
		options = append(options, jwtv5.WithIssuer(a.issuer))
	}
	if a.audience != "" {
		options = append(options, jwtv5.WithAudience(a.audience))
	}
	return options
}

// Issue mints an HS256 token for subject carrying custom claims for ttl.
func (a *adapter) Issue(_ context.Context, subject string, custom map[string]any, ttl time.Duration) (auth.Token, error) {
	if subject == "" {
		return auth.Token{}, fmt.Errorf("jwt: issue: %w: empty subject", auth.ErrInvalidToken)
	}
	if ttl <= 0 {
		return auth.Token{}, fmt.Errorf("jwt: issue: %w: non-positive ttl", auth.ErrInvalidToken)
	}
	if a.maxTTL > 0 && ttl > a.maxTTL {
		return auth.Token{}, fmt.Errorf("jwt: issue: %w: ttl %v exceeds max %v", auth.ErrInvalidToken, ttl, a.maxTTL)
	}
	for k := range custom {
		if _, clash := registeredKeys[k]; clash {
			return auth.Token{}, fmt.Errorf("jwt: issue: %w: custom claim %q collides with registered claim", auth.ErrInvalidToken, k)
		}
	}

	var jtiBytes [16]byte
	if _, err := io.ReadFull(randReader, jtiBytes[:]); err != nil {
		return auth.Token{}, fmt.Errorf("jwt: issue: jti: %w", err)
	}

	now := time.Now()
	claims := jwtv5.MapClaims{
		"jti": hex.EncodeToString(jtiBytes[:]),
		"sub": subject,
		"exp": jwtv5.NewNumericDate(now.Add(ttl)),
		"iat": jwtv5.NewNumericDate(now),
	}
	if a.issuer != "" {
		claims["iss"] = a.issuer
	}
	if a.audience != "" {
		claims["aud"] = a.audience
	}
	for k, v := range custom {
		claims[k] = auth.CloneValue(v)
	}

	// HMAC-SHA256 over a validated []byte secret with JSON-serializable
	// MapClaims: signing cannot fail.
	raw, _ := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims).SignedString(a.secret)
	return auth.Token{Value: raw, ExpiresAt: now.Add(ttl)}, nil
}

// Verify authenticates token and returns its claims.
func (a *adapter) Verify(ctx context.Context, token string) (auth.Claims, error) {
	if token == "" {
		return auth.Claims{}, auth.ErrInvalidToken
	}

	parsed, err := jwtv5.ParseWithClaims(token, jwtv5.MapClaims{}, a.keyfunc, a.parserOptions()...)
	if err != nil {
		if errors.Is(err, jwtv5.ErrTokenExpired) {
			return auth.Claims{}, auth.ErrTokenExpired
		}
		return auth.Claims{}, fmt.Errorf("jwt: verify: %w", auth.ErrInvalidToken)
	}
	// ParseWithClaims into &MapClaims{} guarantees concrete type;
	// nil error implies parsed.Valid.
	claims, _ := parsed.Claims.(jwtv5.MapClaims)

	// Strict empty-side: a token carrying iss/aud the adapter does not
	// configure is rejected (cross-tenant bypass block).
	iss, _ := claims.GetIssuer()
	if a.issuer == "" && iss != "" {
		return auth.Claims{}, fmt.Errorf("jwt: verify: %w: unexpected issuer", auth.ErrInvalidToken)
	}
	aud, _ := claims.GetAudience()
	if a.audience == "" && len(aud) > 0 {
		return auth.Claims{}, fmt.Errorf("jwt: verify: %w: unexpected audience", auth.ErrInvalidToken)
	}

	subject, _ := claims.GetSubject()
	exp, _ := claims.GetExpirationTime()
	var expiresAt time.Time
	if exp != nil {
		expiresAt = exp.Time
	}

	if jti, _ := claims["jti"].(string); jti != "" {
		revoked, err := a.revocation.IsRevoked(ctx, jti)
		if err != nil {
			return auth.Claims{}, fmt.Errorf("jwt: verify: %w", err)
		}
		if revoked {
			return auth.Claims{}, auth.ErrTokenRevoked
		}
	}

	custom := make(map[string]any, len(claims))
	for k, v := range claims {
		if _, reg := registeredKeys[k]; reg {
			continue
		}
		custom[k] = auth.CloneValue(v)
	}
	if len(custom) == 0 {
		custom = nil
	}

	return auth.Claims{Subject: subject, Custom: custom, ExpiresAt: expiresAt}, nil
}

// Revoke invalidates token after verifying its signature. Idempotent:
// revoking an already-revoked token returns nil. A token carrying no jti
// claim cannot be revoked individually and is rejected with ErrInvalidToken
// wrapping revocation.ErrNoJTI: adapter-issued tokens always carry a jti
// (see Issue), so this only rejects hand-crafted or foreign tokens.
func (a *adapter) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("jwt: revoke: %w: empty token", auth.ErrInvalidToken)
	}

	// Signature only: claims are not validated so already-expired tokens
	// can still be recorded (stored with a 1s grace expiry below).
	parsed, err := jwtv5.ParseWithClaims(token, jwtv5.MapClaims{}, a.keyfunc,
		jwtv5.WithValidMethods([]string{"HS256"}),
		jwtv5.WithoutClaimsValidation(),
	)
	if err != nil || !parsed.Valid {
		return fmt.Errorf("jwt: revoke: %w", auth.ErrInvalidToken)
	}
	// Same parser guarantee as Verify: ParseWithClaims into &MapClaims{}
	// ensures concrete type; nil error + parsed.Valid already checked above.
	claims, _ := parsed.Claims.(jwtv5.MapClaims)

	jti, _ := claims["jti"].(string)
	if jti == "" {
		return fmt.Errorf("jwt: revoke: %w: %w", auth.ErrInvalidToken, revocation.ErrNoJTI)
	}

	now := time.Now()
	until := now.Add(time.Second)
	if exp, _ := claims.GetExpirationTime(); exp != nil && exp.After(now) {
		until = exp.Time
	}

	if err := a.revocation.Revoke(ctx, jti, until); err != nil {
		return fmt.Errorf("jwt: revoke: %w", err)
	}
	return nil
}

// Close releases the revocation store's resources. Idempotent.
func (a *adapter) Close() error {
	return a.revocation.Close()
}
