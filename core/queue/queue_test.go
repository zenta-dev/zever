package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var queueAdapterSeq atomic.Int64

func freshQueueAdapter() Adapter { return Adapter(fmt.Sprintf("test-%d", 1000+queueAdapterSeq.Add(1))) }

type stubQueue struct{}

var _ Queue = stubQueue{}

func (stubQueue) Push(_ context.Context, _ string, _ Payload, _ Headers) error { return nil }

func (stubQueue) PushDelayed(_ context.Context, _ string, _ Payload, _ Headers, _ time.Duration) error {
	return nil
}

func (stubQueue) Pop(_ context.Context, _ string) (Message, error) { return Message{}, nil }

func (stubQueue) Ack(_ context.Context, _ Message) error { return nil }

func (stubQueue) Nack(_ context.Context, _ Message, _ bool) error { return nil }

func (stubQueue) Length(_ context.Context, _ string) (int64, error) { return 0, nil }

func (stubQueue) IsEmpty(_ context.Context, _ string) (bool, error) { return true, nil }

func (stubQueue) Close() error { return nil }

func (stubQueue) Name() string { return "" }

func TestQueueOpen_registeredFactory_returnsQueue(t *testing.T) {
	a := freshQueueAdapter()
	wantOpts := Options{VisibilityTimeout: time.Minute}
	var gotOpts Options
	factory := func(opts Options) (Queue, error) {
		gotOpts = opts
		return stubQueue{}, nil
	}
	if err := Register(a, factory); err != nil {
		t.Fatalf("Register(%v) = %v, want nil", a, err)
	}
	q, err := Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open(%v) = %v, want nil", a, err)
	}
	if q == nil {
		t.Fatalf("Open(%v) returned nil Queue", a)
	}
	if gotOpts != wantOpts {
		t.Fatalf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestQueueRegister_nilFactory(t *testing.T) {
	a := freshQueueAdapter()
	err := Register(a, nil)
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register(nil) = %v, want ErrNilFactory", err)
	}
}

func TestQueueRegister_duplicate(t *testing.T) {
	a := freshQueueAdapter()
	stub := func(Options) (Queue, error) { return stubQueue{}, nil }
	if err := Register(a, stub); err != nil {
		t.Fatalf("first Register(%v) = %v, want nil", a, err)
	}
	err := Register(a, stub)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register(%v) = %v, want ErrDuplicate", a, err)
	}
	var dupErr *DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(%v, *DuplicateError) = false", err)
	}
	if dupErr.Adapter != a {
		t.Fatalf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

func TestQueueOpen_unknown(t *testing.T) {
	_, err := Open(Adapter("test-9999"), Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open(unknown) = %v, want ErrUnknownAdapter", err)
	}
	var unkErr *UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(%v, *UnknownAdapterError) = false", err)
	}
}

func TestQueueOpen_factoryError_wrapped(t *testing.T) {
	a := freshQueueAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Queue, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register(%v) = %v, want nil", a, err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open factory error = %v, want sentinel %v", err, sentinel)
	}
	if !strings.Contains(err.Error(), "queue: open") {
		t.Fatalf("Open factory error %q does not contain %q", err.Error(), "queue: open")
	}
}
