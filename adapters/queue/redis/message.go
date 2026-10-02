package redis

import (
	"bytes"

	"encoding/json/v2"

	"github.com/zenta-dev/zever/core/queue"
)

type wireMessage struct {
	ID       string        `json:"id"`
	Payload  queue.Payload `json:"payload"`
	Headers  wireHeaders   `json:"headers"`
	Attempt  int           `json:"attempt"`
	PoppedAt int64         `json:"popped_at,omitempty"`
}

// wireHeaders mirrors queue.Headers on the wire. Its custom decoder accepts
// the empty JSON array miniredis Lua scripts produce for empty objects:
// miniredis encodes an empty Lua table as `[]`, so a stored `"headers":{ }`
// comes back as `"headers":[]` after claim/nack/reclaim round-trips. Real
// Redis preserves `{ }`; both decode to empty here. Null and missing stay
// nil.
type wireHeaders queue.Headers

// UnmarshalJSON decodes headers from an object, null, or the empty array
// fake-Lua form. A non-empty array is invalid and returns an error.
func (h *wireHeaders) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("[]")) {
		*h = nil

		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return err
	}

	*h = wireHeaders(m)

	return nil
}

func toWireMessage(m queue.Message) wireMessage {
	return wireMessage{
		ID:      m.ID.String(),
		Payload: m.Payload.Clone(),
		Headers: wireHeaders(m.Headers.Clone()),
		Attempt: m.Attempt,
	}
}
