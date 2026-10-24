// Package sms is a third-party SMS battery template.
//
// Copy this module out of the zever checkout, rename it, and publish it
// from your own repo. It proves an out-of-repo author can ship a battery
// using only public modules: config, container, and shared/codec.
// Backends are selected in code: the host imports this package and calls
// New directly (then rebuilds to change backends). No zever internals, no
// workspace membership, no network I/O.
package sms

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/shared/codec"
)

// SMS sends short text messages.
type SMS interface {
	// Send delivers body to to.
	Send(ctx context.Context, to string, body string) error
}

// PluginName is the config.Plugins key and container plugin name.
const PluginName = "sms"

// Options configures SMS backend construction.
type Options struct {
	// From is the sender identity (phone number or short code).
	From string `json:"from,omitzero"`
}

// Validate checks options for consistency.
func (o Options) Validate() error {
	if o.From == "" {
		return &InvalidOptionsError{Reason: "from must be non-empty"}
	}
	return nil
}

// ErrInvalidOptions is returned for invalid options or send arguments.
var ErrInvalidOptions = errors.New("sms: invalid options")

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

// New validates opts and returns a stub SMS backend. Hosts wire it in
// code: call it directly or from a plugin build func, then rebuild to
// change backends.
func New(opts Options) (SMS, error) {
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

// validatePluginOptions implements config.RegisterPluginValidator: it decodes
// the plugin's raw options and runs Options.Validate. Unknown fields pass
// through shared/codec untouched until here. Backend selection is code
// (BuildFromConfig calls New), never config.
func validatePluginOptions(opts jsontext.Value) error {
	var o Options
	if len(opts) > 0 {
		if err := codec.Unmarshal(opts, &o); err != nil {
			return err
		}
	}
	return o.Validate()
}

// BuildFromConfig decodes cfg.Plugins["sms"] and constructs the stub
// backend in code. It is the container.Plugin build func registered by
// RegisterPlugin.
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
		if err := codec.Unmarshal(entry.Options, &opts); err != nil {
			return nil, fmt.Errorf("sms: decode options: %w", err)
		}
	}
	return New(opts)
}

// RegisterPlugin wires the full path in one call for copy-paste hosts:
// battery registration (config validator) plus container plugin (versioned
// build func reading cfg.Plugins["sms"]).
// Call once at startup before container.Resolve.
func RegisterPlugin() error {
	if err := config.RegisterPluginValidator(PluginName, validatePluginOptions); err != nil {
		return fmt.Errorf("sms: register config validator: %w", err)
	}
	if err := container.RegisterPlugin[SMS](PluginName, container.PluginAPIVersion, BuildFromConfig); err != nil {
		return fmt.Errorf("sms: register container plugin: %w", err)
	}
	return nil
}
