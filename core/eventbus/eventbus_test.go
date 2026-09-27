package eventbus

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+int(freshSeq.Add(1))))
}

type stubBus struct{}

func (stubBus) Publish(_ context.Context, _ string, _ Payload, _ Headers) error {
	return nil
}

func (stubBus) Subscribe(_ context.Context, _ string, _ Handler) (func(), error) {
	return func() {}, nil
}

func (stubBus) Close() error { return nil }

func (stubBus) Name() string { return "stub" }

func (stubBus) SubscribeChan(_ context.Context, _ string, _ int) (<-chan Message, error) {
	return nil, ErrNotSubscribed
}

func (stubBus) Unsubscribe(_ string, _ <-chan Message) error { return ErrNotSubscribed }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (EventBus, error) { return stubBus{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	if err := Register(a, ok); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := Adapter("test-9999")
	_, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v want %v", ue.Adapter, a)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (EventBus, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "eventbus: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "eventbus: open")
	}
}

func TestOpen_success_returnsBus(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (EventBus, error) { return stubBus{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	b, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if perr := b.Publish(t.Context(), "t", NewPayload([]byte("hi")), NewHeaders(nil)); perr != nil {
		t.Fatalf("Publish err = %v", perr)
	}
	unsub, serr := b.Subscribe(t.Context(), "t", func(context.Context, Message) {})
	if serr != nil {
		t.Fatalf("Subscribe err = %v", serr)
	}
	unsub()
	if cerr := b.Close(); cerr != nil {
		t.Fatalf("Close err = %v", cerr)
	}
}
