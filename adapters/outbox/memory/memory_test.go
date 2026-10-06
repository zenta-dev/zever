package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/adapters/outbox/memory"
	"github.com/zenta-dev/zever/core/outbox"
)

func TestRecordPublishesImmediately(t *testing.T) {
	t.Parallel()

	var got []outbox.Message

	pub := outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
		got = append(got, m)

		return nil
	})

	s, err := memory.New(memory.Options{Publisher: pub})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if err := s.Record(context.Background(), nil, outbox.Message{ID: "1", Topic: "t", Payload: []byte("x")}); err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("published %d messages, want 1", len(got))
	}

	if string(got[0].Payload) != "x" {
		t.Errorf("Payload = %q, want x", got[0].Payload)
	}
}

func TestRecordValidatesMessage(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{Publisher: outbox.PublisherFunc(func(context.Context, outbox.Message) error { return nil })})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Record(context.Background(), nil, outbox.Message{}); !errors.Is(err, outbox.ErrInvalidMessage) {
		t.Fatalf("Record(invalid) = %v, want ErrInvalidMessage", err)
	}
}

func TestRecordWithoutPublisher(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = s.Record(context.Background(), nil, outbox.Message{ID: "1", Topic: "t"})
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("Record(nil publisher) = %v, want ErrInvalidOptions", err)
	}
}

func TestNoOps(t *testing.T) {
	t.Parallel()

	s, err := memory.New(memory.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	if st := s.Status(); st != (outbox.Status{}) {
		t.Errorf("Status() = %+v, want zero", st)
	}

	if s.Name() != "memory" {
		t.Errorf("Name() = %q, want memory", s.Name())
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	if _, err := memory.New(memory.Options{Options: outbox.Options{Table: "bad name"}}); !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("New(bad options) = %v, want ErrInvalidOptions", err)
	}
}

func TestRegister(t *testing.T) {
	memory.Register()

	s, err := outbox.Open(outbox.Memory, outbox.Options{})
	if err != nil {
		t.Fatalf("Open(memory) error = %v", err)
	}

	defer func() { _ = s.Close() }()

	if s.Name() != "memory" {
		t.Errorf("Name() = %q, want memory", s.Name())
	}
}
