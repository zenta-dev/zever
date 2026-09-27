package session

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/shared/registry"
)

// Session is a server-side session record.
type Session struct {
	// ID uniquely identifies the session. It is a secret.
	ID string `json:"id" toml:"id" yaml:"id"`
	// Data holds arbitrary session values.
	Data map[string]any `json:"data" toml:"data" yaml:"data"`
	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at" toml:"created_at" yaml:"created_at"`
	// UpdatedAt is when the session was last saved.
	UpdatedAt time.Time `json:"updated_at" toml:"updated_at" yaml:"updated_at"`
	// ExpiresAt is when the session expires. Zero means no expiry.
	ExpiresAt time.Time `json:"expires_at" toml:"expires_at" yaml:"expires_at"`
}

// NewSession creates a Session with id, empty data, and lifetimes
// anchored at now. A ttl <= 0 leaves ExpiresAt zero (no expiry).
func NewSession(id string, ttl time.Duration) Session {
	now := time.Now()
	s := Session{
		ID:        id,
		Data:      make(map[string]any),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if ttl > 0 {
		s.ExpiresAt = now.Add(ttl)
	}
	return s
}

// Clone returns a deep copy of s; mutating the copy never affects the original.
func (s Session) Clone() Session {
	c := s
	c.Data = cloneData(s.Data)
	return c
}

// Expired reports whether s is expired at now.
// A zero ExpiresAt never expires; expiry itself counts as expired.
func (s Session) Expired(now time.Time) bool {
	if s.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(s.ExpiresAt)
}

func cloneData(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	cp := make(map[string]any, len(m))
	for k, v := range m {
		cp[k] = cloneValue(v)
	}
	return cp
}

func cloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneData(val)
	case map[string]string:
		cp := make(map[string]string, len(val))
		for k, vv := range val {
			cp[k] = vv
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, vv := range val {
			cp[i] = cloneValue(vv)
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

// Store is the session backend.
//
// Create mints a fresh ID and expiry and returns the new session.
// Get returns the session for id, or ErrNotFound on miss or expiry
// (expired sessions are indistinguishable from missing ones).
// Save upserts sess and touches UpdatedAt. Delete removes id and is
// idempotent (nil on missing IDs).
type Store interface {
	// Create mints a fresh session with the given ttl.
	Create(ctx context.Context, ttl time.Duration) (Session, error)
	// Get returns the session for id.
	Get(ctx context.Context, id string) (Session, error)
	// Save upserts sess and touches UpdatedAt.
	Save(ctx context.Context, sess Session) error
	// Delete removes id; missing IDs return nil.
	Delete(ctx context.Context, id string) error
	// Close shuts down the store and releases associated resources.
	Close() error
}

// Factory creates a Store from the given Options.
type Factory func(opts Options) (Store, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates a Store for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Store, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("session: open %s: %w", adapter, err)
	}

	return s, nil
}
