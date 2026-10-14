// Package jwt provides an HS256 JWT authentication adapter.
//
// HS256 only; asymmetric algorithms (EdDSA, RS256) are not supported.
// Deployments that need them must supply a custom auth.Auth adapter.
//
// Custom claims are merged at the top level of the token alongside the
// registered claims (sub, iss, aud, exp, iat, jti). Custom keys colliding
// with a registered claim name are rejected at Issue time.
//
// Key rotation: configure auth.JWTKey entries plus ActiveKID (empty
// ActiveKID signs with Keys[0]). Issued tokens carry the signing key ID in
// the "kid" header; verification selects the key by "kid" and falls back
// to the legacy Secret for kid-less tokens. Keep retired keys listed until
// every token signed with them has expired. Unknown kids fail closed.
// Rotation is HMAC-only: asymmetric algorithms (EdDSA, RS256) are
// deliberately unsupported, and the keyfunc rejects any token whose alg
// header is not exactly "HS256" (plus jwt.WithValidMethods), blocking
// none/RS256-confusion attacks.
//
// Time claims: exp is required and nbf, when present, is enforced; both
// honor the configured leeway for clock skew (zero default means strict;
// operators with skewed fleets typically set ~30s).
//
// Revocation: the default in-process memory store is per-process —
// revocations are lost on restart and invisible to other instances.
// Multi-instance deployments must inject a shared revocation.Store
// (core/auth/revocation/redis, or a custom implementation) via
// auth.JWTOptions.RevocationStore. New logs a one-time slog WARN when the
// default store is used, and the adapter exposes the same text through
// WarningReporter.
//
// Security notes:
//   - HS256 only: the keyfunc rejects any token whose alg header is not
//     exactly "HS256", and the parser is additionally constrained with
//     jwt.WithValidMethods. This blocks none/RS256-confusion attacks.
//   - issuer/audience: when configured they are enforced via
//     jwt.WithIssuer/WithAudience. When empty, tokens carrying an
//     issuer/audience are rejected to avoid cross-tenant bypass when the
//     same HMAC secret is shared across tenants.
//
// Timestamps (exp, iat) are second-precision on the wire: NumericDate
// serializes as unix seconds, so sub-second TTLs round down and expiry
// checks resolve to whole seconds. Keep TTLs >= 1s.
//
// Proposal (not implemented -- wire change): a sub-second-precision
// envelope would need a versioned claim or a new token version, breaking
// every issued token, so it stays a proposal until a token versioning
// scheme exists.
package jwt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/auth/revocation"
	revocationmemory "github.com/zenta-dev/zever/core/auth/revocation/memory"
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
	// legacy is the kid-less HMAC secret from opts.JWT.Secret, nil when
	// rotation keys alone are configured. It verifies pre-rotation
	// tokens; new tokens always carry a kid when keys exist.
	legacy []byte
	// keys maps kid to a private copy of that key's HMAC secret.
	keys map[string][]byte
	// activeKID and active select the signing key. activeKID is empty
	// (and active is the legacy secret) when no Keys are configured.
	activeKID string
	active    []byte
	issuer    string
	audience  string
	maxTTL    time.Duration
	leeway    time.Duration

	// parserOpts and revokeOpts are computed once at construction: the
	// adapter's validation configuration is immutable after New, and
	// rebuilding the option slices (plus their closures) on every
	// Verify/Revoke call only adds allocations.
	parserOpts []jwtv5.ParserOption
	revokeOpts []jwtv5.ParserOption

	revocation revocation.Store
	warnings   []string
	closed     atomic.Bool
}

// memoryRevocationWarning is logged once per adapter, and returned by
// Warnings, when the default in-memory revocation store is in use.
const memoryRevocationWarning = "jwt: using the default in-memory revocation store: " +
	"revocations are per-process and lost on restart; " +
	"inject a shared store (core/auth/revocation/redis or a custom revocation.Store) " +
	"via auth.JWTOptions.RevocationStore for multi-instance or durable revocation"

// WarningReporter is implemented by the adapter returned from New. It
// lets doctor-style checks surface configuration hazards that are not
// errors, such as the per-process default revocation store.
type WarningReporter interface {
	// Warnings returns a copy of the startup warnings; empty when none.
	Warnings() []string
}

// Warnings returns a copy of the adapter's startup warnings. It is safe
// for concurrent use.
func (a *adapter) Warnings() []string {
	return append([]string(nil), a.warnings...)
}

// New builds an HS256 JWT Auth from opts.
//
// opts is validated with auth.Options.Validate first (wrapped as "jwt: %w").
// At least one of the legacy JWT secret or a Keys entry is required: an
// empty secret with no keys (or a short secret) returns a
// *auth.InvalidOptionsError. With Keys configured, ActiveKID selects the
// signing key and defaults to Keys[0] when empty.
func New(opts auth.Options) (auth.Auth, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}
	secret := opts.JWT.Secret
	if secret == "" && len(opts.JWT.Keys) == 0 {
		return nil, auth.InvalidOptionsError{Reason: "jwt secret is required"}
	}
	if secret != "" && len([]byte(secret)) < auth.MinSecretLen {
		return nil, auth.InvalidOptionsError{
			Reason: fmt.Sprintf("jwt secret must be at least %d bytes", auth.MinSecretLen),
		}
	}

	keys := make(map[string][]byte, len(opts.JWT.Keys))
	for _, k := range opts.JWT.Keys {
		keys[k.KID] = append([]byte(nil), []byte(k.Secret)...)
	}
	var legacy, active []byte
	var activeKID string
	if secret != "" {
		legacy = append([]byte(nil), []byte(secret)...)
		active = legacy
	}
	if len(keys) > 0 {
		activeKID = opts.JWT.ActiveKID
		if activeKID == "" {
			activeKID = opts.JWT.Keys[0].KID
		}
		active = keys[activeKID]
	}

	store := opts.JWT.RevocationStore
	var warnings []string
	if store == nil {
		warnings = append(warnings, memoryRevocationWarning)
		slog.Warn(memoryRevocationWarning)
		// memory.New with zero Options performs no I/O and cannot fail:
		// the store allocates no external resources. It is per-process:
		// multi-instance deployments must inject a shared
		// revocation.Store (see the package doc).
		store, _ = revocationmemory.New(revocationmemory.Options{})
	}

	a := &adapter{
		legacy:     legacy,
		keys:       keys,
		activeKID:  activeKID,
		active:     active,
		issuer:     opts.JWT.Issuer,
		audience:   opts.JWT.Audience,
		maxTTL:     opts.JWT.MaxTTL,
		leeway:     opts.JWT.Leeway,
		revocation: store,
		warnings:   warnings,
	}
	a.parserOpts = a.parserOptions()
	a.revokeOpts = []jwtv5.ParserOption{
		jwtv5.WithValidMethods([]string{"HS256"}),
		jwtv5.WithoutClaimsValidation(),
	}
	return a, nil
}

// keyfunc enforces HS256 with an exact, case-sensitive alg comparison,
// then selects the HMAC secret by the token's kid header: known kids use
// their rotation key, kid-less tokens fall back to the legacy secret, and
// unknown kids (or kid-less tokens with no legacy secret) fail closed.
func (a *adapter) keyfunc(t *jwtv5.Token) (any, error) {
	if t.Method.Alg() != "HS256" {
		return nil, fmt.Errorf("jwt: unexpected signing method %q", t.Method.Alg())
	}
	if kid, _ := t.Header["kid"].(string); kid != "" {
		secret, ok := a.keys[kid]
		if !ok {
			return nil, fmt.Errorf("jwt: unknown key id %q: %w", kid, auth.ErrInvalidToken)
		}
		return secret, nil
	}
	if a.legacy == nil {
		return nil, fmt.Errorf("jwt: token carries no key id: %w", auth.ErrInvalidToken)
	}
	return a.legacy, nil
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
	if a.leeway > 0 {
		options = append(options, jwtv5.WithLeeway(a.leeway))
	}
	return options
}

// closedErr reports use after Close. It joins auth.ErrInvalidToken so
// callers matching on the existing error taxonomy keep failing closed
// without learning a new sentinel.
func (a *adapter) closedErr(op string) error {
	return fmt.Errorf("jwt: %s: %w: adapter closed", op, auth.ErrInvalidToken)
}

// Issue mints an HS256 token for subject carrying custom claims for ttl.
func (a *adapter) Issue(_ context.Context, subject string, custom map[string]any, ttl time.Duration) (auth.Token, error) {
	if a.closed.Load() {
		return auth.Token{}, a.closedErr("issue")
	}
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

	// HMAC-SHA256 over a validated []byte secret. Custom claims must be
	// JSON-serializable; unserializable values fail here instead of
	// minting an empty token. The token carries the active key's kid
	// header when rotation keys are configured; legacy (single-secret)
	// tokens stay kid-less.
	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)
	if a.activeKID != "" {
		token.Header["kid"] = a.activeKID
	}
	raw, err := token.SignedString(a.active)
	if err != nil {
		return auth.Token{}, fmt.Errorf("jwt: issue: signing failed: %w", errors.Join(auth.ErrInvalidToken, err))
	}
	return auth.Token{Value: raw, ExpiresAt: now.Add(ttl)}, nil
}

// Verify authenticates token and returns its claims.
func (a *adapter) Verify(ctx context.Context, token string) (auth.Claims, error) {
	if a.closed.Load() {
		return auth.Claims{}, a.closedErr("verify")
	}
	if token == "" {
		return auth.Claims{}, auth.ErrInvalidToken
	}

	parsed, err := jwtv5.ParseWithClaims(token, jwtv5.MapClaims{}, a.keyfunc, a.parserOpts...)
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
	// Issue always sets a non-empty subject, but a manually-signed token
	// sharing the secret could omit sub and otherwise verify: an empty
	// subject must never authenticate.
	if subject == "" {
		return auth.Claims{}, fmt.Errorf("jwt: verify: %w: empty subject", auth.ErrInvalidToken)
	}
	exp, _ := claims.GetExpirationTime()
	var expiresAt time.Time
	if exp != nil {
		expiresAt = exp.Time
	}

	if rawJTI, present := claims["jti"]; present {
		jti, ok := rawJTI.(string)
		if !ok {
			return auth.Claims{}, fmt.Errorf("jwt: verify: %w: non-string jti claim", auth.ErrInvalidToken)
		}
		if jti != "" {
			revoked, err := a.revocation.IsRevoked(ctx, jti)
			if err != nil {
				return auth.Claims{}, fmt.Errorf("jwt: verify: %w", err)
			}
			if revoked {
				return auth.Claims{}, auth.ErrTokenRevoked
			}
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
	if a.closed.Load() {
		return a.closedErr("revoke")
	}
	if token == "" {
		return fmt.Errorf("jwt: revoke: %w: empty token", auth.ErrInvalidToken)
	}

	// Signature only: claims are not validated so already-expired tokens
	// can still be recorded (stored with a 1s grace expiry below).
	parsed, err := jwtv5.ParseWithClaims(token, jwtv5.MapClaims{}, a.keyfunc, a.revokeOpts...)
	if err != nil || !parsed.Valid {
		return fmt.Errorf("jwt: revoke: %w", auth.ErrInvalidToken)
	}
	// Same parser guarantee as Verify: ParseWithClaims into &MapClaims{}
	// ensures concrete type; nil error + parsed.Valid already checked above.
	claims, _ := parsed.Claims.(jwtv5.MapClaims)

	jti, _ := claims["jti"].(string)
	if jti == "" {
		return fmt.Errorf("jwt: revoke: %w", errors.Join(auth.ErrInvalidToken, revocation.ErrNoJTI))
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

// Close releases the revocation store's resources and zeroes every HMAC
// secret copy (legacy plus rotation keys) so key material does not linger
// after shutdown. Idempotent: signing and verification fail closed once
// closed.
func (a *adapter) Close() error {
	a.closed.Store(true)
	clear(a.legacy)
	for _, secret := range a.keys {
		clear(secret)
	}
	return a.revocation.Close()
}
