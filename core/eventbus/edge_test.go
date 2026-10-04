package eventbus

import (
	"errors"
	"strings"
	"testing"
)

func TestEdgeValidateTopic_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		topic   string
		wantErr bool
	}{
		{"empty", "", true},
		{"single byte", "a", false},
		{"max length", strings.Repeat("a", MaxTopicLen), false},
		{"over max", strings.Repeat("a", MaxTopicLen+1), true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateTopic(tc.topic)

			var ioe *InvalidOptionsError
			if tc.wantErr && !errors.As(err, &ioe) {
				t.Fatalf("validateTopic(len=%d) err = %v, want *InvalidOptionsError", len(tc.topic), err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("validateTopic(len=%d) err = %v, want nil", len(tc.topic), err)
			}
		})
	}
}

func TestEdgeMessage_Clone_nilFields(t *testing.T) {
	t.Parallel()

	m := Message{Topic: "t"}
	c := m.Clone()

	if c.Payload != nil {
		t.Fatalf("Clone payload = %v, want nil", c.Payload)
	}

	if c.Headers != nil {
		t.Fatalf("Clone headers = %v, want nil", c.Headers)
	}
}

func TestEdgePayloadClone_nil(t *testing.T) {
	t.Parallel()

	var p Payload
	if got := p.Clone(); got != nil {
		t.Fatalf("nil Payload.Clone() = %v, want nil", got)
	}
}

func TestEdgeHeadersClone_nil(t *testing.T) {
	t.Parallel()

	var h Headers
	if got := h.Clone(); got != nil {
		t.Fatalf("nil Headers.Clone() = %v, want nil", got)
	}
}
