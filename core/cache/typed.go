package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/shared/codec"
)

// Key constrains the key types usable with Typed.
type Key interface {
	~string | ~int8 | ~int16 | ~int32 | ~int64 | ~float32 | ~float64
}

// Typed provides a type-safe cache over a Cache backend using a Codec for encoding values.
type Typed[K Key, V any] struct {
	backend Cache
	codec   codec.Codec[V]
}

// NewTyped creates a Typed cache over backend using codec for value encoding.
func NewTyped[K Key, V any](backend Cache, codec codec.Codec[V]) *Typed[K, V] {
	return &Typed[K, V]{backend: backend, codec: codec}
}

func (t *Typed[K, V]) key(k K) string {
	return fmt.Sprint(k)
}

// Get retrieves and decodes the value stored under k.
func (t *Typed[K, V]) Get(ctx context.Context, k K) (V, error) {
	var zero V

	raw, err := t.backend.Get(ctx, t.key(k))
	if err != nil {
		return zero, fmt.Errorf("cache: typed get %v: %w", k, err)
	}

	v, err := t.codec.Decode(raw)
	if err != nil {
		return zero, fmt.Errorf("cache: Decode value for key %v: %w", k, err)
	}

	return v, nil
}

// Set encodes v and stores it under k.
func (t *Typed[K, V]) Set(ctx context.Context, k K, v V, ttl time.Duration) error {
	raw, err := t.codec.Encode(v)
	if err != nil {
		return fmt.Errorf("cache: Encode value for key %v: %w", k, err)
	}

	if err := t.backend.Set(ctx, t.key(k), raw, ttl); err != nil {
		return fmt.Errorf("cache: typed set %v: %w", k, err)
	}
	return nil
}

// SetIfAbsent encodes v and stores it under k only when no live entry exists.
func (t *Typed[K, V]) SetIfAbsent(ctx context.Context, k K, v V, ttl time.Duration) (bool, error) {
	raw, err := t.codec.Encode(v)
	if err != nil {
		return false, fmt.Errorf("cache: Encode value for key %v: %w", k, err)
	}

	ok, err := t.backend.SetIfAbsent(ctx, t.key(k), raw, ttl)
	if err != nil {
		return false, fmt.Errorf("cache: typed setifabsent %v: %w", k, err)
	}
	return ok, nil
}

// Delete removes the entry stored under k.
func (t *Typed[K, V]) Delete(ctx context.Context, k K) error {
	if err := t.backend.Delete(ctx, t.key(k)); err != nil {
		return fmt.Errorf("cache: typed delete %v: %w", k, err)
	}
	return nil
}

// Increment atomically increments the integer value stored under k.
func (t *Typed[K, V]) Increment(ctx context.Context, k K) error {
	if err := t.backend.Increment(ctx, t.key(k)); err != nil {
		return fmt.Errorf("cache: typed increment %v: %w", k, err)
	}
	return nil
}

// Decrement atomically decrements the integer value stored under k.
func (t *Typed[K, V]) Decrement(ctx context.Context, k K) error {
	if err := t.backend.Decrement(ctx, t.key(k)); err != nil {
		return fmt.Errorf("cache: typed decrement %v: %w", k, err)
	}
	return nil
}

// Exists reports whether a live entry exists under k.
func (t *Typed[K, V]) Exists(ctx context.Context, k K) (bool, error) {
	ok, err := t.backend.Exists(ctx, t.key(k))
	if err != nil {
		return false, fmt.Errorf("cache: typed exists %v: %w", k, err)
	}
	return ok, nil
}

// Close releases the underlying backend resources.
func (t *Typed[K, V]) Close(ctx context.Context) error {
	if err := t.backend.Close(ctx); err != nil {
		return fmt.Errorf("cache: typed close: %w", err)
	}
	return nil
}
