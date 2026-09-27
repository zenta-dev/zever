// Package sms is a third-party SMS battery template.
//
// Copy this module out of the zever checkout, rename it, and publish it
// from your own repo. It proves an out-of-repo author can ship a battery
// using only public modules: config, container, and shared/registry.
// No zever internals, no workspace membership, no network I/O.
package sms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/shared/registry"
)

// SMS sends short text messages.
type SMS interface {
	// Send delivers body to to.
	Send(ctx context.Context, to string, body string) error
}

// Adapter identifies the SMS backend implementation.
type Adapter string

const (
	// Stub selects the deterministic in-memory stub SMS backend.
	Stub Adapter = "stub"
)

// PluginName is the config.Plugins key and container plugin name.
const PluginName = "sms"

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses an adapter name. Any non-empty name is accepted so
// custom backends round-trip through config file and env selection.
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

// Sentinel errors. All typed errors unwrap to these for errors.Is.
var (
	// ErrNilFactory is returned when an adapter factory is nil.
	ErrNilFactory = errors.New("sms: nil factory")
	// ErrDuplicate is returned on duplicate adapter registration.
	ErrDuplicate = errors.New("sms: duplicate registration")
	// ErrUnknownAdapter is returned for an unregistered adapter.
	ErrUnknownAdapter = errors.New("sms: unknown adapter")
	// ErrInvalidAdapter is returned for an invalid adapter name.
	ErrInvalidAdapter = errors.New("sms: invalid adapter")
	// ErrInvalidOptions is returned for invalid options or send arguments.
	ErrInvalidOptions = errors.New("sms: invalid options")
)

// DuplicateAdapterError reports a duplicate adapter registration.
type DuplicateAdapterError struct {
	// Adapter is the already-registered adapter.
	Adapter Adapter
}

// Error returns a human-readable duplicate-registration message.
func (e *DuplicateAdapterError) Error() string {
	return fmt.Sprintf("%s: %s", ErrDuplicate, e.Adapter.String())
}

// Unwrap returns ErrDuplicate.
func (e *DuplicateAdapterError) Unwrap() error { return ErrDuplicate }

// UnknownAdapterError reports a lookup of an unregistered adapter.
type UnknownAdapterError struct {
	// Adapter is the unregistered adapter.
	Adapter Adapter
}

// Error returns a human-readable unknown-adapter message.
func (e *UnknownAdapterError) Error() string {
	return fmt.Sprintf("%s: %s (forgotten import?)", ErrUnknownAdapter, e.Adapter.String())
}

// Unwrap returns ErrUnknownAdapter.
func (e *UnknownAdapterError) Unwrap() error { return ErrUnknownAdapter }

// InvalidAdapterError reports an invalid adapter name.
type InvalidAdapterError struct {
	// Adapter is the invalid adapter name.
	Adapter string
}

// Error returns a human-readable invalid-adapter message.
func (e *InvalidAdapterError) Error() string {
	return fmt.Sprintf("%s: %q", ErrInvalidAdapter, e.Adapter)
}

// Unwrap returns ErrInvalidAdapter.
func (e *InvalidAdapterError) Unwrap() error { return ErrInvalidAdapter }

// InvalidOptionsError reports an SMS options validation failure.
type InvalidOptionsError struct {
	// Reason describes the validation failure.
	Reason string
}

// Error returns a human-readable invalid-options message.
func (e *InvalidOptionsError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidOptions, e.Reason)
}

// Unwrap returns ErrInvalidOptions.
func (e *InvalidOptionsError) Unwrap() error { return ErrInvalidOptions }

// Factory creates an SMS backend from Options.
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

// Message is one recorded Send call.
type Message struct {
	// To is the recipient.
	To string
	// Body is the message body.
	Body string
}

var _ SMS = (*stubDriver)(nil)

// stubDriver is the deterministic in-memory SMS backend. It performs no
// network I/O and is safe for concurrent use.
type stubDriver struct {
	mu   sync.Mutex
	from string
	sent []Message
}

// stubOpen validates opts and returns a stub SMS backend.
func stubOpen(opts Options) (SMS, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("sms stub: %w", err)
	}
	return &stubDriver{from: opts.From}, nil
}

// Send records a message. Empty to/body fail with ErrInvalidOptions.
func (d *stubDriver) Send(_ context.Context, to string, body string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if to == "" {
		return fmt.Errorf("sms stub: send: %w: to must be non-empty", ErrInvalidOptions)
	}
	if body == "" {
		return fmt.Errorf("sms stub: send: %w: body must be non-empty", ErrInvalidOptions)
	}
	d.sent = append(d.sent, Message{To: to, Body: body})
	return nil
}

// Sent returns a copy of recorded messages.
func (d *stubDriver) Sent() []Message {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]Message, len(d.sent))
	copy(out, d.sent)
	return out
}

// From returns the configured sender identity.
func (d *stubDriver) From() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.from
}

// RegisterStub registers the stub factory under Stub for later Open.
// Call once at startup; duplicate registration returns an error.
func RegisterStub() error {
	return Register(Stub, stubOpen)
}

// validatePluginOptions implements config.RegisterPluginValidator: it decodes
// the plugin's raw options and runs Options.Validate. Unknown fields pass
// through encoding/json untouched until here.
func validatePluginOptions(adapter string, opts json.RawMessage) error {
	if _, err := ParseAdapter(adapter); err != nil {
		return err
	}
	var o Options
	if len(opts) > 0 {
		if err := json.Unmarshal(opts, &o); err != nil {
			return err
		}
	}
	return o.Validate()
}

// BuildFromConfig decodes cfg.Plugins["sms"] and opens the selected adapter.
// It is the container.Plugin build func registered by RegisterPlugin.
func BuildFromConfig(cfg *config.Config) (SMS, error) {
	if cfg == nil {
		return nil, fmt.Errorf("sms: %w: nil config", ErrInvalidOptions)
	}
	entry, ok := cfg.Plugins[PluginName]
	if !ok {
		return nil, fmt.Errorf("sms: %w: missing plugins[%q] entry", ErrInvalidOptions, PluginName)
	}
	var opts Options
	if len(entry.Options) > 0 {
		if err := json.Unmarshal(entry.Options, &opts); err != nil {
			return nil, fmt.Errorf("sms: decode options: %w", err)
		}
	}
	adapter, err := ParseAdapter(entry.Adapter)
	if err != nil {
		return nil, err
	}
	return Open(adapter, opts)
}

// RegisterPlugin wires the full path in one call for copy-paste hosts:
// adapter registration (stub) + battery registration (config validator) +
// container plugin (versioned build func reading cfg.Plugins["sms"]).
// Call once at startup before container.Resolve.
func RegisterPlugin() error {
	if err := RegisterStub(); err != nil {
		return fmt.Errorf("sms: register stub: %w", err)
	}
	if err := config.RegisterPluginValidator(PluginName, validatePluginOptions); err != nil {
		return fmt.Errorf("sms: register config validator: %w", err)
	}
	if err := container.RegisterPlugin[SMS](PluginName, container.PluginAPIVersion, BuildFromConfig); err != nil {
		return fmt.Errorf("sms: register container plugin: %w", err)
	}
	return nil
}
