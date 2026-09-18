// Package registry provides a shared generic factory registry used to dedupe
// Register/Open boilerplate across adapter batteries.
package registry

import (
	"reflect"
	"sync"
)

// Registry is a concurrency-safe map from adapter keys to factories.
//
// A is the adapter key type, F is the factory func type. Per-package error
// strings stay compatible because callers inject their own sentinel and
// constructors; Registry only stores, guards duplicates, and reports misses.
type Registry[A comparable, F any] struct {
	mu        sync.RWMutex
	factories map[A]F
	errNil    error
	duplicate func(A) error
	unknown   func(A) error
}

// New returns an empty Registry that reports errNil on nil factories,
// duplicate(adapter) on repeated Register, and unknown(adapter) on Lookup
// misses. Any hook may be nil, in which case that path returns nil lookup
// semantics are preserved but no custom error is produced.
func New[A comparable, F any](errNil error, duplicate func(A) error, unknown func(A) error) *Registry[A, F] {
	return &Registry[A, F]{
		factories: make(map[A]F),
		errNil:    errNil,
		duplicate: duplicate,
		unknown:   unknown,
	}
}

// Register associates adapter with factory for later Lookup.
func (r *Registry[A, F]) Register(adapter A, factory F) error {
	if isNilFactory(factory) {
		return r.errNil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, dup := r.factories[adapter]; dup {
		if r.duplicate == nil {
			return nil
		}

		return r.duplicate(adapter)
	}

	r.factories[adapter] = factory

	return nil
}

// Lookup returns the factory registered for adapter, or the injected unknown
// error when adapter was never registered.
func (r *Registry[A, F]) Lookup(adapter A) (F, error) {
	r.mu.RLock()
	factory, ok := r.factories[adapter]
	r.mu.RUnlock()

	if !ok {
		var zero F
		if r.unknown == nil {
			return zero, nil
		}

		return zero, r.unknown(adapter)
	}

	return factory, nil
}

// isNilFactory reports whether factory is nil. F is typically a func type,
// where a typed nil wrapped in any is non-nil, so reflect is needed. The
// check runs only on the Register cold path.
func isNilFactory[F any](factory F) bool {
	if any(factory) == nil {
		return true
	}

	v := reflect.ValueOf(factory)
	switch v.Kind() {
	case reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
