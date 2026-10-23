// Package sms is a dogfood example SMS battery implemented as a plugin.
//
// It mirrors the cache/billing/mailer battery shape (interface plus Options
// plus Factory) so plugin authors can copy it. Backend selection is
// compile-time: the app imports the backend package and calls its
// constructor directly (for example stub.Open), then rebuilds to change it.
// There is no adapter registry and no name-based open. The stub backend is
// deterministic and performs no network I/O.
package sms

import (
	"context"
	"errors"
	"fmt"
)

// SMS sends short text messages and releases backend resources on Close.
type SMS interface {
	// Send delivers body to to.
	Send(ctx context.Context, to string, body string) error
	// Close releases backend resources.
	Close(ctx context.Context) error
}

// Options configures SMS backend construction.
type Options struct {
	// From is the sender identity (phone number or short code).
	From string `json:"from"`
}

// Validate checks options for consistency.
func (o Options) Validate() error {
	if o.From == "" {
		return &InvalidOptionsError{Reason: "from must be non-empty"}
	}
	return nil
}

// ErrInvalidOptions is returned for invalid SMS options or send arguments.
var ErrInvalidOptions = errors.New("sms: invalid options")

// ErrClosed is returned when the SMS backend is closed.
var ErrClosed = errors.New("sms: closed")

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
