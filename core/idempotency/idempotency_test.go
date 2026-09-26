package idempotency

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var freshAdapterCounter int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+atomic.AddInt64(&freshAdapterCounter, 1)))
}

func TestRegister_nil_factory_fails(t *testing.T) {
	a := freshAdapter()
	err := Register(a, nil)
	if err == nil {
		t.Fatal("Register(nil) expected error, got nil")
	}
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_fails(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, func(Options) (Store, error) { return &stubStore{}, nil })
	if err == nil {
		t.Fatal("duplicate Register expected error, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("duplicate Register err type = %T, want *DuplicateError", err)
	}
	if de.Adapter != a {
		t.Fatalf("DuplicateError.Adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknown_adapter_fails(t *testing.T) {
	a := freshAdapter()
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open unknown expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("Open unknown err type = %T, want *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("UnknownAdapterError.Adapter = %v, want %v", ue.Adapter, a)
	}
}

func TestOpen_factory_error_wraps(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Store, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open expected factory error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrapped sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "idempotency: open") {
		t.Fatalf("Open err = %q, want prefix %q", err.Error(), "idempotency: open")
	}
}

func TestOpen_registered_success(t *testing.T) {
	a := freshAdapter()
	want := &stubStore{}
	if err := Register(a, func(Options) (Store, error) { return want, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if got != want {
		t.Fatal("Open did not return factory store")
	}
}
