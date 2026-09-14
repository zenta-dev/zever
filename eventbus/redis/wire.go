package redis

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zenta-dev/zever/eventbus"
)

// wireMessage is the JSON envelope stored on the Redis channel.
// Payload marshals as base64 automatically via encoding/json.
type wireMessage struct {
	ID      string            `json:"id"`
	Payload []byte            `json:"payload"`
	Headers map[string]string `json:"headers"`
}

func decodeMessage(topic string, raw []byte) (eventbus.Message, error) {
	if len(raw) > eventbus.MaxMessageSize {
		return eventbus.Message{}, fmt.Errorf("%w: %d > %d", eventbus.ErrPayloadTooLarge, len(raw), eventbus.MaxMessageSize)
	}

	var wm wireMessage
	if err := json.Unmarshal(raw, &wm); err != nil {
		return eventbus.Message{}, fmt.Errorf("redis: decode message: %w", err)
	}

	id, err := eventbus.ParseMessageID(wm.ID)
	if err != nil {
		return eventbus.Message{}, err
	}

	return eventbus.Message{
		ID:      id,
		Topic:   topic,
		Payload: wm.Payload,
		Headers: wm.Headers,
		// Publish time does not cross the wire (envelope is
		// id/payload/headers); stamp receive time instead.
		ReceivedAt: time.Now(),
	}, nil
}
