package notification

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/zenta-dev/zever/internal/registry"
)

// Channel identifies the notification delivery channel.
type Channel string

const (
	// ChannelPush delivers to a push token.
	ChannelPush Channel = "push"
	// ChannelSMS delivers to an E.164 phone number.
	ChannelSMS Channel = "sms"
)

// Priority identifies notification urgency.
type Priority string

const (
	// PriorityLow marks low urgency.
	PriorityLow Priority = "low"
	// PriorityNormal marks normal urgency.
	PriorityNormal Priority = "normal"
	// PriorityHigh marks high urgency.
	PriorityHigh Priority = "high"
)

// Notification is a notification message.
type Notification struct {
	// Target is the push token (push) or E.164 phone number (sms).
	Target string
	// Channel is the delivery channel.
	Channel Channel
	// Title is the notification title (push only).
	Title string
	// Body is the notification body.
	Body string
	// Data carries push key-value payload (push only).
	Data map[string]string
	// Priority is the urgency. Zero value means Normal.
	Priority Priority
	// TTL is the time-to-live. Zero means none.
	TTL time.Duration
}

// NewNotification builds a Notification with target, channel, and body.
// Priority is left zero (means Normal) and TTL zero.
func NewNotification(target string, channel Channel, body string) Notification {
	return Notification{
		Target:  target,
		Channel: channel,
		Body:    body,
	}
}

// Clone returns a deep copy of n, duplicating the Data map.
func (n Notification) Clone() Notification {
	out := n
	if n.Data != nil {
		out.Data = make(map[string]string, len(n.Data))
		for k, v := range n.Data {
			out.Data[k] = v
		}
	}
	return out
}

var e164Re = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// Validate checks the notification shape.
// Target problems return ErrInvalidTarget without echoing the target (PII).
// Unknown channels return ErrInvalidChannel.
// Priority, TTL, and title/data misuse return ErrInvalidNotification.
func (n Notification) Validate() error {
	switch n.Channel {
	case ChannelPush, ChannelSMS:
	default:
		return &InvalidChannelError{Channel: string(n.Channel)}
	}

	if n.Target == "" {
		return &InvalidTargetError{Reason: "target must be non-empty"}
	}
	if n.Channel == ChannelSMS && !e164Re.MatchString(n.Target) {
		return &InvalidTargetError{Reason: "sms target must be E.164"}
	}

	switch n.Priority {
	case "", PriorityLow, PriorityNormal, PriorityHigh:
	default:
		return &InvalidNotificationError{Reason: "priority must be low, normal, or high"}
	}

	if n.TTL < 0 {
		return &InvalidNotificationError{Reason: "ttl must be >= 0"}
	}

	if n.Channel == ChannelSMS {
		if n.Title != "" {
			return &InvalidNotificationError{Reason: "title is push-only"}
		}
		if len(n.Data) > 0 {
			return &InvalidNotificationError{Reason: "data is push-only"}
		}
	}

	return nil
}

// Notifier defines the core operations for sending notifications.
type Notifier interface {
	// Notify delivers n.
	Notify(ctx context.Context, n *Notification) error
	// Close shuts down the notifier and releases associated resources.
	Close() error
}

// Factory creates a Notifier from the given Options.
type Factory func(opts Options) (Notifier, error)

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

// Open creates a Notifier for adapter using the registered Factory and opts.
func Open(adapter Adapter, opts Options) (Notifier, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}

	n, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("notification: open %s: %w", adapter, err)
	}

	return n, nil
}
