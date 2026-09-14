package eventbus

import (
	"errors"
	"testing"
)

func TestMessage_newMessage_assignsFreshIDs(t *testing.T) {
	t.Parallel()
	a := NewMessage("orders", NewPayload([]byte("hi")), NewHeaders(map[string]string{"k": "v"}))
	b := NewMessage("orders", NewPayload([]byte("hi")), NewHeaders(map[string]string{"k": "v"}))
	if a.ID == (MessageID{}) {
		t.Error("NewMessage ID is zero value")
	}
	if b.ID == (MessageID{}) {
		t.Error("NewMessage ID is zero value")
	}
	if a.ID == b.ID {
		t.Errorf("NewMessage IDs not distinct: %v", a.ID)
	}
	if a.Topic != "orders" {
		t.Errorf("NewMessage Topic = %q want %q", a.Topic, "orders")
	}
}

func TestMessage_clone_isDeepCopy(t *testing.T) {
	t.Parallel()
	orig := NewMessage("orders", NewPayload([]byte{1, 2, 3}), NewHeaders(map[string]string{"k": "v"}))
	cloned := orig.Clone()
	if cloned.ID != orig.ID {
		t.Errorf("Clone ID = %v want %v", cloned.ID, orig.ID)
	}
	if cloned.Topic != orig.Topic {
		t.Errorf("Clone Topic = %q want %q", cloned.Topic, orig.Topic)
	}
	cloned.Payload[0] = 99
	cloned.Headers["k"] = "mutated"
	cloned.Headers["extra"] = "x"
	if orig.Payload[0] != 1 {
		t.Errorf("Clone payload deep copy failed: orig.Payload[0]=%d want 1", orig.Payload[0])
	}
	if orig.Headers["k"] != "v" {
		t.Errorf("Clone headers deep copy failed: orig.Headers[k]=%q want %q", orig.Headers["k"], "v")
	}
	if _, ok := orig.Headers["extra"]; ok {
		t.Error("Clone headers extra key leaked to original")
	}
	orig.Payload[1] = 77
	if cloned.Payload[1] != 2 {
		t.Errorf("Clone reverse isolation failed: cloned.Payload[1]=%d want 2", cloned.Payload[1])
	}
}

func TestMessage_parseMessageID_roundtrip(t *testing.T) {
	t.Parallel()
	id := newMessageID()
	parsed, err := ParseMessageID(id.String())
	if err != nil {
		t.Fatalf("ParseMessageID(%q) err = %v", id.String(), err)
	}
	if parsed != id {
		t.Errorf("roundtrip MessageID = %v want %v", parsed, id)
	}
}

func TestMessage_parseMessageID_invalid(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "not-a-uuid", "123", "orders"} {
		_, err := ParseMessageID(in)
		if err == nil {
			t.Errorf("ParseMessageID(%q) expected error, got nil", in)
			continue
		}
		var ime *InvalidMessageIDError
		if !errors.As(err, &ime) {
			t.Errorf("ParseMessageID(%q) err %T is not *InvalidMessageIDError", in, err)
			continue
		}
		if ime.ID != in {
			t.Errorf("ParseMessageID(%q) carried ID = %q", in, ime.ID)
		}
	}
}

func TestHeaders_clone_isDeepCopy(t *testing.T) {
	t.Parallel()
	orig := NewHeaders(map[string]string{"a": "1", "b": "2"})
	cloned := orig.Clone()
	orig["a"] = "99"
	orig["c"] = "3"
	if cloned["a"] != "1" {
		t.Errorf("Headers.Clone deep copy failed: cloned[a]=%q want %q", cloned["a"], "1")
	}
	if _, ok := cloned["c"]; ok {
		t.Errorf("Headers.Clone deep copy failed: cloned contains c=%q want absent", cloned["c"])
	}
}

func TestPayload_clone_isDeepCopy(t *testing.T) {
	t.Parallel()
	orig := NewPayload([]byte{1, 2, 3})
	cloned := orig.Clone()
	orig[0] = 99
	if cloned[0] != 1 {
		t.Errorf("Payload.Clone deep copy failed: cloned[0]=%d want 1", cloned[0])
	}
	cloned[1] = 88
	if orig[1] != 2 {
		t.Errorf("Payload.Clone aliasing: orig[1]=%d want 2", orig[1])
	}
}
