package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
)

type stubStore struct {
	name string
}

func (s *stubStore) Record(context.Context, db.Tx, outbox.Message) error { return nil }
func (s *stubStore) Start(context.Context) error                         { return nil }
func (s *stubStore) Status() outbox.Status                               { return outbox.Status{} }
func (s *stubStore) Close() error                                        { return nil }
func (s *stubStore) Name() string                                        { return s.name }

func TestOpenUnknownAdapter(t *testing.T) {
	_, err := outbox.Open(outbox.Adapter("nope"), outbox.Options{})
	if !errors.Is(err, outbox.ErrUnknownAdapter) {
		t.Fatalf("err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	_, err := outbox.Open(outbox.Adapter("anything"), outbox.Options{Table: "bad name"})
	if !errors.Is(err, outbox.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestRegisterAndOpen(t *testing.T) {
	adapter := outbox.Adapter("stub-open")

	if err := outbox.Register(adapter, func(outbox.Options) (outbox.Store, error) {
		return &stubStore{name: "stub"}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	s, err := outbox.Open(adapter, outbox.Options{})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if s.Name() != "stub" {
		t.Errorf("Name() = %q, want stub", s.Name())
	}
}

func TestRegisterDuplicate(t *testing.T) {
	adapter := outbox.Adapter("stub-dup")

	factory := func(outbox.Options) (outbox.Store, error) { return &stubStore{name: "stub"}, nil }

	if err := outbox.Register(adapter, factory); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	if err := outbox.Register(adapter, factory); !errors.Is(err, outbox.ErrDuplicateAdapter) {
		t.Fatalf("second Register() = %v, want ErrDuplicateAdapter", err)
	}
}

func TestRegisterNilFactory(t *testing.T) {
	if err := outbox.Register(outbox.Adapter("stub-nil"), nil); !errors.Is(err, outbox.ErrNilFactory) {
		t.Fatalf("Register(nil) = %v, want ErrNilFactory", err)
	}
}

func TestRegisterSharedAndOpenShared(t *testing.T) {
	adapter := outbox.Adapter("stub-shared")

	if err := outbox.RegisterShared(adapter, func(db.DB, outbox.Options) (outbox.Store, error) {
		return &stubStore{name: "shared"}, nil
	}); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	s, err := outbox.OpenShared(adapter, nil, outbox.Options{})
	if err != nil {
		t.Fatalf("OpenShared() error = %v", err)
	}

	if s.Name() != "shared" {
		t.Errorf("Name() = %q, want shared", s.Name())
	}
}

func TestRegisterSharedNilFactory(t *testing.T) {
	if err := outbox.RegisterShared(outbox.Adapter("stub-shared-nil"), nil); !errors.Is(err, outbox.ErrNilFactory) {
		t.Fatalf("RegisterShared(nil) = %v, want ErrNilFactory", err)
	}
}

func TestPublisherFunc(t *testing.T) {
	var got outbox.Message

	var p outbox.Publisher = outbox.PublisherFunc(func(_ context.Context, m outbox.Message) error {
		got = m

		return nil
	})

	if err := p.Publish(context.Background(), outbox.Message{ID: "1", Topic: "t"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if got.ID != "1" {
		t.Errorf("got ID = %q, want 1", got.ID)
	}
}
