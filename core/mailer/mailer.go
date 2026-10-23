package mailer

import (
	"context"
)

// Mailer defines the core operations for sending mail.
type Mailer interface {
	// Send delivers msg.
	Send(ctx context.Context, msg *Mail) error
	// Close shuts down the mailer and releases associated resources.
	Close() error
}

// Factory creates a Mailer from the given Options.
type Factory func(opts Options) (Mailer, error)
