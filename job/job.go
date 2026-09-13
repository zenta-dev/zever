package job

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"
	"sync"
)

var (
	headerJobName  = "job_name"
	headerUniqueID = "unique_id"
	headerAttempt  = "attempt"
)

type (
	// Payload carries the raw JSON-encoded arguments for a job handler.
	Payload []byte
	// Handler executes a job with the given context and payload.
	Handler func(ctx context.Context, payload Payload) error
	// Middleware wraps a Handler to add cross-cutting behavior.
	Middleware func(next Handler) Handler
)

var (
	mu          sync.RWMutex
	definitions = make(map[string]Definition)
	middleware  []Middleware
)

// Reset clears all registered job definitions, middleware, and batch callbacks.
func Reset() {
	mu.Lock()
	defer mu.Unlock()

	definitions = make(map[string]Definition)
	middleware = nil

	batchCallbacks.Lock()
	batchCallbacks.m = make(map[string]*batchCallback)
	batchCallbacks.Unlock()
}

// Use appends a Middleware to the global chain applied to job handlers.
func Use(m Middleware) error {
	mu.Lock()
	defer mu.Unlock()

	if m == nil {
		return ErrUseMiddlewareNil
	}

	middleware = append(middleware, m)

	return nil
}

// Register adds a named typed job handler with optional Options. It trims the name, defaults to DefaultRetryPolicy and PriorityLow, and wraps the handler with JSON payload decoding.
func Register[T any](name string, handle func(ctx context.Context, args T) error, opts ...Option) error {
	mu.Lock()
	defer mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return ErrRegisterNameEmpty
	}

	if handle == nil {
		return ErrRegisterHandleNil
	}

	if _, dup := definitions[name]; dup {
		return &DuplicateJobError{Name: name}
	}

	def := Definition{policy: DefaultRetryPolicy(), priority: PriorityLow}

	for _, o := range opts {
		o(&def)
	}

	def.handler = func(ctx context.Context, payload Payload) error {
		var args T
		if err := json.Unmarshal(payload, &args); err != nil {
			return fmt.Errorf("job: %q decode payload: %w", name, err)
		}

		return handle(ctx, args)
	}

	definitions[name] = def

	return nil
}

// Lookup returns the Definition registered under the given name.
func Lookup(name string) (Definition, bool) {
	mu.RLock()
	defer mu.RUnlock()

	d, ok := definitions[name]

	return d, ok
}
