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
	mrand "math/rand"
	"sync"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"

	"github.com/zenta-dev/zever/auth"
)

// randReader is the CSPRNG source for JTIs, swappable in tests to
// simulate host failure. Production always uses crypto/rand.
var randReader = rand.Reader

// newTicker constructs the pruner ticker, swappable in tests.
var newTicker = time.NewTicker

// maxRevoked caps revocation-map memory (~5 MB at ~500 B/token).
const maxRevoked = 10000

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

	mu      sync.RWMutex
	revoked map[string]time.Time
	order   []string
	head    int

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
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

	a := &adapter{
		secret:   append([]byte(nil), []byte(secret)...),
		issuer:   opts.JWT.Issuer,
		audience: opts.JWT.Audience,
		maxTTL:   opts.JWT.MaxTTL,
		revoked:  make(map[string]time.Time),
		done:     make(chan struct{}),
	}
	a.startPruner()
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
func (a *adapter) Verify(_ context.Context, token string) (auth.Claims, error) {
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

	a.mu.RLock()
	_, revoked := a.revoked[token]
	a.mu.RUnlock()
	if revoked {
		return auth.Claims{}, auth.ErrTokenRevoked
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
// revoking an already-revoked token returns nil.
func (a *adapter) Revoke(_ context.Context, token string) error {
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

	now := time.Now()
	until := now.Add(time.Second)
	if exp, _ := claims.GetExpirationTime(); exp != nil && exp.After(now) {
		until = exp.Time
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if _, dup := a.revoked[token]; dup {
		return nil
	}
	a.ensureCapacityLocked(now)
	a.revoked[token] = until
	a.order = append(a.order, token)
	return nil
}

// Close stops the background pruner and releases resources. Idempotent.
func (a *adapter) Close() error {
	a.closeOnce.Do(func() { close(a.done) })
	a.wg.Wait()
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.revoked)
	return nil
}

func (a *adapter) ensureCapacityLocked(now time.Time) {
	if len(a.revoked) < maxRevoked {
		return
	}
	a.pruneExpiredLocked(now)
	if len(a.revoked) >= maxRevoked {
		a.evictOldestLocked()
	}
}

func (a *adapter) pruneExpiredLocked(now time.Time) {
	for tok, until := range a.revoked {
		if !now.Before(until) {
			delete(a.revoked, tok)
		}
	}
}

func (a *adapter) evictOldestLocked() {
	for a.head < len(a.order) {
		tok := a.order[a.head]
		a.head++
		if _, ok := a.revoked[tok]; ok {
			delete(a.revoked, tok)
			break
		}
	}
	if a.head > 1024 && a.head > len(a.order)/2 {
		a.order = append([]string(nil), a.order[a.head:]...)
		a.head = 0
	}
}

func (a *adapter) startPruner() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() { _ = recover() }()
		// Jitter the 1m interval to avoid thundering herd when many
		// adapters start together (e.g. rolling deploy).
		//nolint:gosec // math/rand suffices for non-security jitter; crypto/rand would also read the test-swappable randReader global.
		jitter := time.Duration(mrand.Int63n(int64(10*time.Second))) - 5*time.Second
		ticker := newTicker(time.Minute + jitter)
		defer ticker.Stop()
		for {
			select {
			case <-a.done:
				return
			case now := <-ticker.C:
				a.pruneOnce(now)
			}
		}
	}()
}

// pruneOnce drops expired revocations under lock. Split from the loop
// for direct unit testing.
func (a *adapter) pruneOnce(now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneExpiredLocked(now)
}
