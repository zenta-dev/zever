// Package sms is a dogfood example SMS battery implemented as a plugin.
//
// It mirrors the cache/billing/mailer battery shape (Adapter registry plus
// Register/Open) so plugin authors can copy it. The stub backend is
// deterministic and performs no network I/O.
package sms

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

// SMS sends short text messages and releases backend resources on Close.
type SMS interface {
	// Send delivers body to to.
	Send(ctx context.Context, to string, body string) error
	// Close releases backend resources.
	Close(ctx context.Context) error
}

// Adapter identifies the SMS backend implementation.
type Adapter string

const (
	// Stub selects the deterministic in-memory stub SMS backend.
	Stub Adapter = "stub"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), &InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}

// Options configures SMS backend construction.
type Options struct {
	// From is the sender identity (phone number or short code).
	From string `json:"from" toml:"from" yaml:"from"`
}

// Validate checks options for consistency.
func (o Options) Validate() error {
	if o.From == "" {
		return &InvalidOptionsError{Reason: "from must be non-empty"}
	}
	return nil
}

// ErrNilFactory is returned when an adapter factory is nil.
var ErrNilFactory = errors.New("sms: nil factory")

// ErrDuplicate is returned on duplicate adapter registration.
var ErrDuplicate = errors.New("sms: duplicate registration")

// ErrDuplicateAdapter aliases ErrDuplicate for compatibility.
var ErrDuplicateAdapter = ErrDuplicate

// ErrUnknownAdapter is returned for an unregistered adapter.
var ErrUnknownAdapter = errors.New("sms: unknown adapter")

// ErrInvalidAdapter is returned for an invalid adapter name.
var ErrInvalidAdapter = errors.New("sms: invalid adapter")

// ErrInvalidOptions is returned for invalid SMS options or send arguments.
var ErrInvalidOptions = errors.New("sms: invalid options")

// ErrClosed is returned when the SMS backend is closed.
var ErrClosed = errors.New("sms: closed")

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// DuplicateError aliases DuplicateAdapterError for compatibility.
type DuplicateError = DuplicateAdapterError

// Error returns a human-readable duplicate-registration message.
func (e DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e DuplicateAdapterError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter.
func (e UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }

// InvalidOptionsError reports an SMS options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }

// Factory creates an SMS backend from the given Options.
type Factory func(opts Options) (SMS, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return &DuplicateAdapterError{Adapter: adapter} },
	func(adapter Adapter) error { return &UnknownAdapterError{Adapter: adapter} },
)

// Register associates an Adapter with a Factory for later use by Open.
func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}

	return factories.Register(adapter, factory)
}

// Open creates an SMS backend for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (SMS, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	s, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("sms: open %s: %w", adapter, err)
	}

	return s, nil
}
