package redis

import (
	"github.com/zenta-dev/zever/queue"
)

type wireMessage struct {
	ID       string        `json:"id"`
	Payload  queue.Payload `json:"payload"`
	Headers  queue.Headers `json:"headers"`
	Attempt  int           `json:"attempt"`
	PoppedAt int64         `json:"popped_at,omitempty"`
}

func toWireMessage(m queue.Message) wireMessage {
	return wireMessage{
		ID:      m.ID.String(),
		Payload: m.Payload.Clone(),
		Headers: m.Headers.Clone(),
		Attempt: m.Attempt,
	}
}
