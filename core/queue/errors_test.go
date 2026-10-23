package queue

import (
	"errors"
	"testing"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrClosed":           {ErrClosed.Error(), "queue: closed"},
		"ErrEmpty":            {ErrEmpty.Error(), "queue: empty"},
		"ErrNilFactory":       {ErrNilFactory.Error(), "queue: nil factory"},
		"ErrDuplicate":        {ErrDuplicate.Error(), "queue: duplicate adapter"},
		"ErrUnknownAdapter":   {ErrUnknownAdapter.Error(), "queue: unknown adapter"},
		"ErrInvalidAdapter":   {ErrInvalidAdapter.Error(), "queue: invalid adapter"},
		"ErrInvalidMessageID": {ErrInvalidMessageID.Error(), "queue: invalid message id"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestParseMessageIDInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseMessageID("not-a-uuid")
	if err == nil {
		t.Fatal("ParseMessageID(not-a-uuid) = nil, want ErrInvalidMessageID")
	}

	if !errors.Is(err, ErrInvalidMessageID) {
		t.Errorf("errors.Is(err, ErrInvalidMessageID) = false (err = %v)", err)
	}

	var invErr InvalidMessageIDError
	if !errors.As(err, &invErr) {
		t.Errorf("errors.As(err, InvalidMessageIDError) = false (err = %T %v)", err, err)
	}

	if invErr.ID != "not-a-uuid" {
		t.Errorf("InvalidMessageIDError.ID = %q, want %q", invErr.ID, "not-a-uuid")
	}
}

func TestEmptyError(t *testing.T) {
	t.Parallel()

	err := EmptyError{Topic: "jobs"}
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("errors.Is(EmptyError, ErrEmpty) = false")
	}

	var emptyErr EmptyError
	if !errors.As(err, &emptyErr) {
		t.Errorf("errors.As(EmptyError) = false")
	}

	if got := err.Error(); got != `queue: empty: topic "jobs"` {
		t.Errorf("EmptyError.Error() = %q, want %q", got, `queue: empty: topic "jobs"`)
	}

	bare := ErrEmpty
	if got := bare.Error(); got != "queue: empty" {
		t.Errorf("ErrEmpty.Error() = %q", got)
	}
}

func TestQueueMessage_helpers(t *testing.T) {
	t.Parallel()

	t.Run("message_id/roundtrip", func(t *testing.T) {
		t.Parallel()
		id := stubMessageID()
		s := id.String()
		parsed, err := ParseMessageID(s)
		if err != nil {
			t.Fatalf("ParseMessageID(%q) err = %v", s, err)
		}
		if parsed != id {
			t.Errorf("roundtrip MessageID = %v, want %v", parsed, id)
		}
	})

	t.Run("headers/clone_deep", func(t *testing.T) {
		t.Parallel()
		orig := NewHeaders(map[string]string{"a": "1", "b": "2"})
		cloned := orig.Clone()
		orig["a"] = "99"
		orig["c"] = "3"
		if cloned["a"] != "1" {
			t.Errorf("Headers.Clone deep copy failed: cloned[a]=%q, want %q", cloned["a"], "1")
		}
		if _, ok := cloned["c"]; ok {
			t.Errorf("Headers.Clone deep copy failed: cloned contains c=%q, want absent", cloned["c"])
		}
		if len(cloned) != 2 {
			t.Errorf("Headers cloned len = %d, want 2", len(cloned))
		}
	})

	t.Run("payload/clone_deep", func(t *testing.T) {
		t.Parallel()
		orig := NewPayload([]byte{1, 2, 3})
		cloned := orig.Clone()
		orig[0] = 99
		if cloned[0] != 1 {
			t.Errorf("Payload.Clone deep copy failed: cloned[0]=%d, want 1", cloned[0])
		}
		if len(cloned) != 3 {
			t.Errorf("Payload cloned len = %d, want 3", len(cloned))
		}
		// mutate cloned not affect original
		cloned[1] = 88
		if orig[1] != 2 {
			t.Errorf("Payload.Clone aliasing: orig[1]=%d, want 2", orig[1])
		}
	})

	t.Run("message/clone_deep", func(t *testing.T) {
		t.Parallel()
		orig := NewMessage("jobs", NewPayload([]byte{1, 2, 3}), NewHeaders(map[string]string{"k": "v"}))
		cloned := orig.Clone()
		// mutate cloned payload/headers
		cloned.Payload[0] = 99
		cloned.Headers["k"] = "mutated"
		cloned.Headers["extra"] = "x"
		if orig.Payload[0] != 1 {
			t.Errorf("Message.Clone payload deep copy failed: orig.Payload[0]=%d, want 1", orig.Payload[0])
		}
		if orig.Headers["k"] != "v" {
			t.Errorf("Message.Clone headers deep copy failed: orig.Headers[k]=%q, want %q", orig.Headers["k"], "v")
		}
		if _, ok := orig.Headers["extra"]; ok {
			t.Errorf("Message.Clone headers extra key leaked to original")
		}
		// also mutate original not affect cloned
		orig.Payload[1] = 77
		if cloned.Payload[1] != 2 {
			t.Errorf("Message.Clone reverse isolation failed: cloned.Payload[1]=%d, want 2", cloned.Payload[1])
		}
	})

	t.Run("message/attempt", func(t *testing.T) {
		t.Parallel()
		msg := NewMessage("jobs", NewPayload([]byte("hi")), NewHeaders(nil))
		if msg.Attempt != 1 {
			t.Errorf("NewMessage Attempt = %d, want 1", msg.Attempt)
		}
		if msg.Topic != "jobs" {
			t.Errorf("NewMessage Topic = %q, want %q", msg.Topic, "jobs")
		}
	})
}
