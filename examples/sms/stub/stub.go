// Package stub provides a deterministic in-memory sms.SMS implementation.
//
// It performs no network I/O. Sent messages are recorded in memory for
// inspection in tests and examples. Safe for concurrent use.
package stub

import (
	"context"
	"fmt"
	"sync"

	"github.com/zenta-dev/zever/examples/sms"
)

var _ sms.SMS = (*driver)(nil)

type driver struct {
	mu     sync.Mutex
	from   string
	sent   []Message
	closed bool
}

// Message is one recorded Send call.
type Message struct {
	// To is the recipient.
	To string
	// Body is the message body.
	Body string
}

// Open validates opts and returns a stub SMS backend.
func Open(opts sms.Options) (sms.SMS, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("stub: %w", err)
	}

	return &driver{from: opts.From}, nil
}

// Register registers the stub factory under sms.Stub for later sms.Open.
// Call once at startup; duplicate registration returns an error.
func Register() error {
	return sms.Register(sms.Stub, Open)
}

// Send records a message. Empty to/body fail with sms.ErrInvalidOptions.
func (d *driver) Send(_ context.Context, to string, body string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return sms.ErrClosed
	}

	if to == "" {
		return fmt.Errorf("stub: send: %w: to must be non-empty", sms.ErrInvalidOptions)
	}

	if body == "" {
		return fmt.Errorf("stub: send: %w: body must be non-empty", sms.ErrInvalidOptions)
	}

	d.sent = append(d.sent, Message{To: to, Body: body})

	return nil
}

// Close marks the driver closed; further Send calls fail.
func (d *driver) Close(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.closed = true

	return nil
}

// Sent returns a copy of recorded messages.
func (d *driver) Sent() []Message {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := make([]Message, len(d.sent))
	copy(out, d.sent)

	return out
}

// From returns the configured sender identity.
func (d *driver) From() string {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.from
}
