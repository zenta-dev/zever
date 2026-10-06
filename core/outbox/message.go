package outbox

import (
	"fmt"
	"maps"
	"slices"
	"time"
)

// MaxIDLen is the maximum allowed message ID length in bytes.
const MaxIDLen = 255

// Message is one outbox event. It is a wire type: every field carries
// json/toml/yaml tags so it round-trips through file configuration and JSON.
type Message struct {
	// ID uniquely identifies the message. Required, at most MaxIDLen bytes.
	ID string `json:"id" toml:"id" yaml:"id"`
	// Topic is the destination topic. Required.
	Topic string `json:"topic" toml:"topic" yaml:"topic"`
	// Key is an optional partition/routing key.
	Key string `json:"key,omitempty" toml:"key,omitempty" yaml:"key,omitempty"`
	// Payload is the raw message body.
	Payload []byte `json:"payload,omitempty" toml:"payload,omitempty" yaml:"payload,omitempty"`
	// Headers holds metadata attached to the message.
	Headers map[string]string `json:"headers,omitempty" toml:"headers,omitempty" yaml:"headers,omitempty"`
	// CreatedAt is the message creation timestamp. Zero means "stamp now"
	// for adapters that persist it.
	CreatedAt time.Time `json:"created_at,omitempty" toml:"created_at,omitempty" yaml:"created_at,omitempty"`
	// Attempts is the delivery attempt count observed by the publisher.
	Attempts int `json:"attempts" toml:"attempts" yaml:"attempts"`
}

// Validate reports whether the message is well-formed: ID and Topic must be
// non-empty and ID must not exceed MaxIDLen bytes. Failures wrap
// ErrInvalidMessage.
func (m Message) Validate() error {
	if m.ID == "" {
		return InvalidMessageError{Reason: "id must be non-empty"}
	}

	if len(m.ID) > MaxIDLen {
		return InvalidMessageError{Reason: fmt.Sprintf("id exceeds %d bytes", MaxIDLen)}
	}

	if m.Topic == "" {
		return InvalidMessageError{Reason: "topic must be non-empty"}
	}

	return nil
}

// Clone returns a deep copy of m with cloned payload and headers.
func (m Message) Clone() Message {
	m.Payload = slices.Clone(m.Payload)
	m.Headers = maps.Clone(m.Headers)

	return m
}
