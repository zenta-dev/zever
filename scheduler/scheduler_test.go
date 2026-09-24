package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/job"
	"github.com/zenta-dev/zever/queue"
)

type stubQueue struct {
	pushes      int
	lastPayload queue.Payload
}

func (s *stubQueue) Push(_ context.Context, _ string, payload queue.Payload, _ queue.Headers) error {
	s.pushes++
	s.lastPayload = payload

	return nil
}

func (s *stubQueue) PushDelayed(_ context.Context, _ string, _ queue.Payload, _ queue.Headers, _ time.Duration) error {
	return nil
}

func (s *stubQueue) Pop(_ context.Context, _ string) (queue.Message, error) {
	return queue.Message{}, queue.ErrEmpty
}

func (s *stubQueue) Ack(_ context.Context, _ queue.Message) error { return nil }

func (s *stubQueue) Nack(_ context.Context, _ queue.Message, _ bool) error { return nil }

func (s *stubQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (s *stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (s *stubQueue) Close() error { return nil }

func (s *stubQueue) Name() string { return "stub" }

// fakeScheduler stands in for the embedded implementation, which lives in
// the child scheduler/embedded package: importing that child from this
// internal (package scheduler) test would be an import cycle, since the
// child imports the parent. These tests only exercise Register/Open
// plumbing, so a minimal fake suffices.
type fakeScheduler struct{}

func (fakeScheduler) Schedule(context.Context, string, string, any) (EntryID, error) {
	return 1, nil
}

func (fakeScheduler) Remove(EntryID) error { return nil }

func (fakeScheduler) Entries() []EntryID { return nil }

func (fakeScheduler) Start() error { return nil }

func (fakeScheduler) Stop() error { return nil }

func (fakeScheduler) Name() string { return "embedded" }

func stubFactoryOpts(d *job.Dispatcher) Options {
	return Options{Dispatcher: d}
}

// TestOptionsErrorString covers InvalidOptionsError.Error, previously
// exercised through the embedded constructor test that moved with the
// implementation into scheduler/embedded.
func TestOptionsErrorString(t *testing.T) {
	t.Parallel()

	err := Options{}.Validate()
	if got, want := err.Error(), "scheduler: invalid options: dispatcher is required"; got != want {
		t.Fatalf("Error()=%q want %q", got, want)
	}
}

func stubFactory(Options) (Scheduler, error) {
	return fakeScheduler{}, nil
}

func TestRegisterNilFactory(t *testing.T) {
	t.Parallel()

	err := Register(Adapter(101), nil)
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("err=%v want ErrNilFactory", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()

	a := Adapter(102)
	d := &job.Dispatcher{Q: &stubQueue{}}

	if err := Register(a, func(Options) (Scheduler, error) {
		return stubFactory(stubFactoryOpts(d))
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := Register(a, func(Options) (Scheduler, error) {
		return stubFactory(stubFactoryOpts(d))
	})

	var dup *DuplicateError
	if !errors.As(err, &dup) {
		t.Fatalf("err=%v want DuplicateError", err)
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("err=%v want ErrDuplicate", err)
	}
}

func TestOpenUnknown(t *testing.T) {
	t.Parallel()

	_, err := Open(Adapter(103), Options{Dispatcher: &job.Dispatcher{}})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("err=%v want ErrUnknownAdapter", err)
	}

	var unk *UnknownAdapterError
	if !errors.As(err, &unk) {
		t.Fatalf("err=%v want UnknownAdapterError", err)
	}
}

func TestOpenFactoryErrorWrapped(t *testing.T) {
	t.Parallel()

	a := Adapter(104)
	sentinel := errors.New("boom")

	if err := Register(a, func(Options) (Scheduler, error) {
		return nil, sentinel
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, err := Open(a, Options{Dispatcher: &job.Dispatcher{}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want wrapped sentinel", err)
	}
}

func TestOpenOk(t *testing.T) {
	t.Parallel()

	a := Adapter(105)
	d := &job.Dispatcher{Q: &stubQueue{}}

	if err := Register(a, func(Options) (Scheduler, error) {
		return stubFactory(stubFactoryOpts(d))
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	s, err := Open(a, stubFactoryOpts(d))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if s.Name() != "embedded" {
		t.Fatalf("Name()=%q want embedded", s.Name())
	}
}
