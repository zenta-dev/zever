package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/session"
)

var _ auth.Auth = (*adapter)(nil)

// expiryLeeway tolerates clock skew between issuer and verifier before an
// envelope expiry counts as expired. One minute covers typical NTP drift
// and container clock skew without materially extending short-lived
// sessions; tokens remain hard-expired by the backing store independently,
// so leeway only affects the envelope check here, never store TTL.
const expiryLeeway = time.Minute

// Envelope keys inside session Data.
const (
	keySubject = "sub"
	keyCustom  = "custom"
	keyExp     = "exp"
)

type adapter struct {
	store session.Store
}

// New builds a session-backed auth.Auth. Core options are validated first.
// The Session.Store is required: a nil store fails with
// *auth.InvalidOptionsError. The adapter never constructs a store itself,
// so it depends only on the session.Store interface, never on a sibling
// adapter package.
func New(opts auth.Options) (auth.Auth, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	store := opts.Session.Store
	if store == nil {
		return nil, &auth.InvalidOptionsError{Reason: "session store is required"}
	}
	return &adapter{store: store}, nil
}

// Issue mints a token for subject carrying claims for ttl.
func (a *adapter) Issue(ctx context.Context, subject string, claims map[string]any, ttl time.Duration) (auth.Token, error) {
	if subject == "" {
		return auth.Token{}, auth.ErrInvalidToken
	}
	if ttl <= 0 {
		return auth.Token{}, fmt.Errorf("session: ttl must be positive, got %v", ttl)
	}
	// Create mints the ID and the authoritative expiry. A bare Save with
	// a fresh ID cannot do this: stores ignore caller ExpiresAt on
	// missing IDs and apply the store default TTL instead, so
	// Create-then-Save is required. Save on the existing record keeps
	// the Created expiry and only fills Data.
	created, err := a.store.Create(ctx, ttl)
	if err != nil {
		return auth.Token{}, fmt.Errorf("session: issue: %w", err)
	}
	created.Data = map[string]any{
		keySubject: subject,
		keyCustom:  cloneCustom(claims),
		keyExp:     created.ExpiresAt.Unix(),
	}
	if err := a.store.Save(ctx, created); err != nil {
		_ = a.store.Delete(ctx, created.ID) // best-effort orphan cleanup
		return auth.Token{}, fmt.Errorf("session: issue: %w", err)
	}
	return auth.Token{Value: created.ID, ExpiresAt: created.ExpiresAt}, nil
}

// Verify authenticates token and returns its claims.
func (a *adapter) Verify(ctx context.Context, token string) (auth.Claims, error) {
	if token == "" {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	if err := session.ValidateID(token); err != nil {
		return auth.Claims{}, fmt.Errorf("session: %w", err)
	}
	sess, err := a.store.Get(ctx, token)
	if err != nil {
		// Miss and store-level expiry are indistinguishable by design.
		if errors.Is(err, session.ErrNotFound) {
			return auth.Claims{}, auth.ErrInvalidToken
		}
		return auth.Claims{}, fmt.Errorf("session: verify: %w", err)
	}
	sub, custom, exp, err := decode(sess.Data)
	if err != nil {
		return auth.Claims{}, auth.ErrInvalidToken
	}
	if time.Now().After(exp.Add(expiryLeeway)) {
		_ = a.store.Delete(ctx, token) // lazy expiry cleanup, best-effort
		return auth.Claims{}, auth.ErrTokenExpired
	}
	return auth.Claims{Subject: sub, Custom: custom, ExpiresAt: exp}, nil
}

// Revoke invalidates token. Deletion is idempotent: unknown IDs return nil
// per the session.Store contract.
func (a *adapter) Revoke(ctx context.Context, token string) error {
	if token == "" {
		return fmt.Errorf("session: revoke: %w: empty token", auth.ErrInvalidToken)
	}
	if err := session.ValidateID(token); err != nil {
		return fmt.Errorf("session: revoke: %w", err)
	}
	if err := a.store.Delete(ctx, token); err != nil {
		return fmt.Errorf("session: revoke: %w", err)
	}
	return nil
}

// Close shuts down the store and releases associated resources.
func (a *adapter) Close() error {
	return a.store.Close()
}

// cloneCustom deep-copies claims via auth.CloneValue; nil stays nil.
func cloneCustom(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = auth.CloneValue(v)
	}
	return cp
}

// decode rebuilds subject, claims, and expiry from the session Data
// envelope, deep-copying custom claims. Missing keys, corrupt types, an
// empty subject, or a non-positive exp all fail.
func decode(data map[string]any) (string, map[string]any, time.Time, error) {
	fail := errors.New("session: corrupt envelope")
	sub, ok := data[keySubject].(string)
	if !ok || sub == "" {
		return "", nil, time.Time{}, fail
	}
	var custom map[string]any
	if raw, ok := data[keyCustom]; ok && raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return "", nil, time.Time{}, fail
		}
		custom = cloneCustom(m)
	}
	// Exp is stored as unix seconds (int64); accept int and float64 too
	// since some stores round-trip Data through JSON.
	var expUnix int64
	switch v := data[keyExp].(type) {
	case int64:
		expUnix = v
	case int:
		expUnix = int64(v)
	case float64:
		expUnix = int64(v)
	default:
		return "", nil, time.Time{}, fail
	}
	if expUnix <= 0 {
		return "", nil, time.Time{}, fail
	}
	return sub, custom, time.Unix(expUnix, 0), nil
}
