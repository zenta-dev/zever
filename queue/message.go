package queue

import (
	"maps"
	"slices"
	"uuid"
)

type (
	// MessageID uniquely identifies a queue message.
	// It is backed by a UUIDv7 value.
	MessageID uuid.UUID
)

func newMessageID() MessageID {
	return MessageID(uuid.NewV7())
}

// ParseMessageID parses id into a MessageID.
// It returns InvalidMessageIDError when id is not a valid UUID.
func ParseMessageID(id string) (MessageID, error) {
	v, err := uuid.Parse(id)
	if err != nil {
		return MessageID{}, &InvalidMessageIDError{ID: id, Err: err}
	}

	return MessageID(v), nil
}

// String returns the canonical string form of the MessageID.
func (id MessageID) String() string {
	return uuid.UUID(id).String()
}

// Headers holds string key-value metadata attached to a message.
type Headers map[string]string

// NewHeaders creates Headers from the given map.
func NewHeaders(v map[string]string) Headers {
	return Headers(v)
}

// Clone returns a deep copy of h.
func (h Headers) Clone() Headers {
	return maps.Clone(h)
}

// Payload holds the raw message body.
type Payload []byte

// NewPayload creates a Payload from the given bytes.
func NewPayload(v []byte) Payload {
	return Payload(v)
}

// Clone returns a deep copy of p.
func (p Payload) Clone() Payload {
	return slices.Clone(p)
}

// Message represents a queued message with identity, routing, and delivery metadata.
type Message struct {
	// ID is the unique identifier for the message.
	ID MessageID
	// Topic is the queue topic where the message resides.
	Topic string
	// Payload is the raw message body.
	Payload Payload
	// Headers holds metadata attached to the message.
	Headers Headers
	// Attempt is the delivery attempt count for the message.
	Attempt int
}

// NewMessage creates a new Message for topic with payload and headers.
// It assigns a fresh MessageID and sets Attempt to 1.
func NewMessage(
	topic string,
	payload Payload,
	headers Headers,
) Message {
	return Message{
		ID:      newMessageID(),
		Topic:   topic,
		Payload: payload,
		Headers: headers,
		Attempt: 1,
	}
}

// Clone returns a deep copy of m with cloned payload and headers.
func (m Message) Clone() Message {
	m.Payload = m.Payload.Clone()
	m.Headers = m.Headers.Clone()

	return m
}
