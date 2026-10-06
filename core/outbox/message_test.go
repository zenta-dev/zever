package outbox_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/outbox"
)

func TestMessageValidate(t *testing.T) {
	t.Parallel()

	if err := (outbox.Message{}).Validate(); !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("empty message err = %v, want ErrInvalidMessage", err)
	}

	valid := outbox.Message{ID: "e1", Topic: "orders", Payload: []byte("{}")}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid message err = %v, want nil", err)
	}

	longID := outbox.Message{ID: strings.Repeat("x", outbox.MaxIDLen+1), Topic: "t"}
	if err := longID.Validate(); !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("long id err = %v, want ErrInvalidMessage", err)
	}

	atLimit := outbox.Message{ID: strings.Repeat("x", outbox.MaxIDLen), Topic: "t"}
	if err := atLimit.Validate(); err != nil {
		t.Fatalf("id at limit err = %v, want nil", err)
	}
}

func TestMessageClone(t *testing.T) {
	t.Parallel()

	orig := outbox.Message{
		ID:      "e1",
		Topic:   "orders",
		Payload: []byte("abc"),
		Headers: map[string]string{"k": "v"},
	}

	clone := orig.Clone()
	clone.Payload[0] = 'X'
	clone.Headers["k"] = "mutated"

	if string(orig.Payload) != "abc" {
		t.Errorf("original payload mutated: %q", orig.Payload)
	}

	if orig.Headers["k"] != "v" {
		t.Errorf("original headers mutated: %q", orig.Headers["k"])
	}
}
