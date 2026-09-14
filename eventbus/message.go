package eventbus

import (
	"maps"
	"slices"
	"time"
	"uuid"
)

type (
	// MessageID uniquely identifies an eventbus message.
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

// Message represents an eventbus message with identity, routing, and metadata.
type Message struct {
	// ID is the unique identifier for the message.
	ID MessageID
	// Topic is the eventbus topic where the message resides.
	Topic string
	// Payload is the raw message body.
	Payload Payload
	// Headers holds metadata attached to the message.
	Headers Headers
	// ReceivedAt is the publish timestamp stamped by NewMessage.
	// Messages decoded from the Redis wire envelope (which carries only
	// id/payload/headers) stamp it at decode time instead.
	ReceivedAt time.Time
}

// NewMessage creates a new Message for topic with payload and headers.
// It assigns a fresh MessageID and stamps ReceivedAt with the current time.
func NewMessage(
	topic string,
	payload Payload,
	headers Headers,
) Message {
	return Message{
		ID:         newMessageID(),
		Topic:      topic,
		Payload:    payload,
		Headers:    headers,
		ReceivedAt: time.Now(),
	}
}

// Clone returns a deep copy of m with cloned payload and headers.
func (m Message) Clone() Message {
	m.Payload = m.Payload.Clone()
	m.Headers = m.Headers.Clone()

	return m
}
