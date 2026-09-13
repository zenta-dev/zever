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
		"ErrDuplicate":        {ErrDuplicate.Error(), "queue: duplicate registration"},
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

	var invErr *InvalidMessageIDError
	if !errors.As(err, &invErr) {
		t.Errorf("errors.As(err, InvalidMessageIDError) = false (err = %T %v)", err, err)
	}

	if invErr != nil && invErr.ID != "not-a-uuid" {
		t.Errorf("InvalidMessageIDError.ID = %q, want %q", invErr.ID, "not-a-uuid")
	}
}

func TestEmptyError(t *testing.T) {
	t.Parallel()

	err := &EmptyError{Topic: "jobs"}
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("errors.Is(EmptyError, ErrEmpty) = false")
	}

	var emptyErr *EmptyError
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

func TestQueueAdapterString_returnsName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		adapter Adapter
		want    string
	}{
		{"memory", Memory, "memory"},
		{"redis", Redis, "redis"},
		{"unknown", Adapter(99), "Adapter(99)"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := c.adapter.String(); got != c.want {
				t.Errorf("Adapter(%d).String() = %q, want %q", int(c.adapter), got, c.want)
			}
		})
	}
}

func TestQueueParseAdapter(t *testing.T) {
	t.Parallel()

	t.Run("valid/memory", func(t *testing.T) {
		t.Parallel()
		got, err := ParseAdapter("memory")
		if err != nil {
			t.Fatalf("ParseAdapter(memory) err = %v, want nil", err)
		}
		if got != Memory {
			t.Errorf("ParseAdapter(memory) = %v, want %v", got, Memory)
		}
	})

	t.Run("valid/redis", func(t *testing.T) {
		t.Parallel()
		got, err := ParseAdapter("redis")
		if err != nil {
			t.Fatalf("ParseAdapter(redis) err = %v, want nil", err)
		}
		if got != Redis {
			t.Errorf("ParseAdapter(redis) = %v, want %v", got, Redis)
		}
	})

	t.Run("invalid/bogus", func(t *testing.T) {
		t.Parallel()
		got, err := ParseAdapter("bogus")
		if err == nil {
			t.Fatal("ParseAdapter(bogus) = nil, want error")
		}
		if got != Memory {
			t.Errorf("ParseAdapter(bogus) got = %v, want %v", got, Memory)
		}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}
		var invErr *InvalidAdapterError
		if !errors.As(err, &invErr) {
			t.Fatalf("errors.As(err, *InvalidAdapterError) = false (err = %T %v)", err, err)
		}
		if invErr.Adapter != "bogus" {
			t.Errorf("InvalidAdapterError.Adapter = %q, want %q", invErr.Adapter, "bogus")
		}
	})
}

func TestQueueTypedErrorMessages_unwrap(t *testing.T) {
	t.Parallel()

	t.Run("empty/with_topic", func(t *testing.T) {
		t.Parallel()
		err := &EmptyError{Topic: "jobs"}
		if got := err.Error(); got != `queue: empty: topic "jobs"` {
			t.Errorf("EmptyError.Error() = %q, want %q", got, `queue: empty: topic "jobs"`)
		}
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("errors.Is(EmptyError with topic, ErrEmpty) = false")
		}
		var target *EmptyError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(err, *EmptyError) = false")
		}
	})

	t.Run("empty/bare", func(t *testing.T) {
		t.Parallel()
		err := &EmptyError{}
		if got := err.Error(); got != "queue: empty" {
			t.Errorf("EmptyError bare Error() = %q, want %q", got, "queue: empty")
		}
		if !errors.Is(err, ErrEmpty) {
			t.Errorf("errors.Is(EmptyError bare, ErrEmpty) = false")
		}
		var target *EmptyError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(bare EmptyError, *EmptyError) = false")
		}
		if got := ErrEmpty.Error(); got != "queue: empty" {
			t.Errorf("ErrEmpty.Error() = %q, want %q", got, "queue: empty")
		}
		if !errors.Is(ErrEmpty, ErrEmpty) {
			t.Errorf("errors.Is(ErrEmpty, ErrEmpty) = false")
		}
	})

	t.Run("invalid_message_id/parse", func(t *testing.T) {
		t.Parallel()
		_, err := ParseMessageID("not-a-uuid")
		if err == nil {
			t.Fatal("ParseMessageID(not-a-uuid) = nil, want error")
		}
		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("errors.Is(err, ErrInvalidMessageID) = false (err = %v)", err)
		}
		var target *InvalidMessageIDError
		if !errors.As(err, &target) {
			t.Fatalf("errors.As(err, *InvalidMessageIDError) = false (err = %T %v)", err, err)
		}
		if target.ID != "not-a-uuid" {
			t.Errorf("InvalidMessageIDError.ID = %q, want %q", target.ID, "not-a-uuid")
		}
	})

	t.Run("invalid_message_id/bare", func(t *testing.T) {
		t.Parallel()
		err := &InvalidMessageIDError{ID: "x"}
		if got := err.Error(); got != `queue: invalid message id "x": <nil>` && got != `queue: invalid message id "x": %!v(<nil>)` {
			if len(got) == 0 || got[:27] != `queue: invalid message id "` {
				t.Errorf("InvalidMessageIDError bare Error() = %q", got)
			}
		}
		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("errors.Is(bare InvalidMessageIDError, ErrInvalidMessageID) = false")
		}
		var target *InvalidMessageIDError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(bare, *InvalidMessageIDError) = false")
		}
		if target != nil && target.ID != "x" {
			t.Errorf("ID = %q, want %q", target.ID, "x")
		}
	})

	t.Run("invalid_message_id/error_string_with_cause", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("parse boom")
		err := &InvalidMessageIDError{ID: "bad-id", Err: cause}
		if got := err.Error(); got != `queue: invalid message id "bad-id": parse boom` {
			t.Errorf("InvalidMessageIDError Error() = %q, want %q", got, `queue: invalid message id "bad-id": parse boom`)
		}
		if !errors.Is(err, ErrInvalidMessageID) {
			t.Errorf("errors.Is false")
		}
		if !errors.Is(err, cause) {
			t.Errorf("errors.Is cause false")
		}
	})

	t.Run("invalid_adapter", func(t *testing.T) {
		t.Parallel()
		err := InvalidAdapterError{Adapter: "bogus"}
		if got := err.Error(); got != `queue: invalid adapter: "bogus"` {
			t.Errorf("InvalidAdapterError.Error() = %q, want %q", got, `queue: invalid adapter: "bogus"`)
		}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Errorf("errors.Is(InvalidAdapterError, ErrInvalidAdapter) = false")
		}
		var target *InvalidAdapterError
		_, perr := ParseAdapter("bogus")
		if !errors.As(perr, &target) {
			t.Errorf("errors.As(ParseAdapter bogus, *InvalidAdapterError) = false")
		}
		perr2 := &InvalidAdapterError{Adapter: "bogus"}
		if !errors.As(perr2, &target) {
			t.Errorf("errors.As(&InvalidAdapterError, *InvalidAdapterError) = false")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		err := &DuplicateError{Adapter: Memory}
		if got := err.Error(); got != "queue: duplicate registration: memory" {
			t.Errorf("DuplicateError.Error() = %q, want %q", got, "queue: duplicate registration: memory")
		}
		if !errors.Is(err, ErrDuplicate) {
			t.Errorf("errors.Is(DuplicateError, ErrDuplicate) = false")
		}
		var target *DuplicateError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(err, *DuplicateError) = false")
		}
		if target != nil && target.Adapter != Memory {
			t.Errorf("DuplicateError.Adapter = %v, want %v", target.Adapter, Memory)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		err := &UnknownAdapterError{Adapter: Adapter(99)}
		if got := err.Error(); got != "queue: unknown adapter: Adapter(99) (forgotten import?)" {
			t.Errorf("UnknownAdapterError.Error() = %q, want %q", got, "queue: unknown adapter: Adapter(99) (forgotten import?)")
		}
		if !errors.Is(err, ErrUnknownAdapter) {
			t.Errorf("errors.Is(UnknownAdapterError, ErrUnknownAdapter) = false")
		}
		var target *UnknownAdapterError
		if !errors.As(err, &target) {
			t.Errorf("errors.As(err, *UnknownAdapterError) = false")
		}
		if target != nil && target.Adapter != Adapter(99) {
			t.Errorf("UnknownAdapterError.Adapter = %v, want %v", target.Adapter, Adapter(99))
		}
	})
}

func TestQueueMessage_helpers(t *testing.T) {
	t.Parallel()

	t.Run("message_id/roundtrip", func(t *testing.T) {
		t.Parallel()
		id := NewMessageIDForTest()
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
