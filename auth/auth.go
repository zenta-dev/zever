package auth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Token is an issued authentication credential.
type Token struct {
	// Value is the opaque credential bytes rendered as a string.
	// It is a secret and must never be logged or echoed in errors.
	Value string
	// ExpiresAt is when the token expires. Zero means no expiry.
	ExpiresAt time.Time
}

// Claims is the verified identity carried by a token.
type Claims struct {
	// Subject is the authenticated principal.
	Subject string
	// Custom holds arbitrary adapter-issued claims.
	Custom map[string]any
	// ExpiresAt is when the claims expire. Zero means no expiry.
	ExpiresAt time.Time
}

// Clone returns a deep copy of c; mutating the copy never affects the original.
func (c Claims) Clone() Claims {
	cp := c
	cp.Custom = cloneCustom(c.Custom)
	return cp
}

func cloneCustom(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = CloneValue(v)
	}
	return cp
}

// CloneValue deep-copies v for adapter use. Maps (map[string]any,
// map[string]string), slices ([]any, []string) and []byte are copied
// recursively; all other values are returned as-is (scalars and other
// reference types are shared).
func CloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for k, vv := range val {
			cp[k] = CloneValue(vv)
		}
		return cp
	case map[string]string:
		cp := make(map[string]string, len(val))
		for k, vv := range val {
			cp[k] = vv
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, vv := range val {
			cp[i] = CloneValue(vv)
		}
		return cp
	case []byte:
		cp := make([]byte, len(val))
		copy(cp, val)
		return cp
	case []string:
		cp := make([]string, len(val))
		copy(cp, val)
		return cp
	default:
		return val
	}
}

// Auth defines the core operations for issuing and verifying credentials.
//
// Semantics: empty subject/token handling is adapter-validated
// (adapters reject with ErrInvalidToken); a ttl <= 0 on Issue is
// adapter-rejected.
type Auth interface {
	// Issue mints a token for subject carrying claims for ttl.
	Issue(ctx context.Context, subject string, claims map[string]any, ttl time.Duration) (Token, error)
	// Verify authenticates token and returns its claims.
	Verify(ctx context.Context, token string) (Claims, error)
	// Revoke invalidates token; unknown tokens return ErrInvalidToken,
	// already-revoked tokens return ErrTokenRevoked.
	Revoke(ctx context.Context, token string) error
	// Close shuts down the backend and releases associated resources.
	Close() error
}

// Factory creates an Auth from the given Options.
type Factory func(opts Options) (Auth, error)

var (
	mu        sync.RWMutex
	factories = make(map[Adapter]Factory)
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, dup := factories[adapter]; dup {
		return &DuplicateError{Adapter: adapter}
	}

	factories[adapter] = factory

	return nil
}

// Open creates an Auth for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Auth, error) {
	mu.RLock()
	factory, ok := factories[adapter]
	mu.RUnlock()

	if !ok {
		return nil, &UnknownAdapterError{Adapter: adapter}
	}

	a, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("auth: open %s: %w", adapter, err)
	}

	return a, nil
}
