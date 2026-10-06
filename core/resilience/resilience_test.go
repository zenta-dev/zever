package resilience_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

var freshSeq atomic.Int64

func freshAdapter() resilience.Adapter {
	return resilience.Adapter(fmt.Sprintf("test-%d", 1000+int(freshSeq.Add(1))))
}

type stubGuard struct {
	name string
	err  error
}

func (s *stubGuard) Execute(ctx context.Context, fn func(context.Context) error) error {
	if s.err != nil {
		return s.err
	}

	return fn(ctx)
}

func (s *stubGuard) State() resilience.State { return resilience.StateClosed }

func (s *stubGuard) Name() string { return s.name }

func (s *stubGuard) Close() error { return nil }

type stubManager struct {
	g resilience.Guard
}

func (m stubManager) Guard(name string) (resilience.Guard, error) {
	if m.g != nil {
		return m.g, nil
	}

	return &stubGuard{name: name}, nil
}

func (m stubManager) Close() error { return nil }

func TestOpenUnknownAdapter(t *testing.T) {
	t.Parallel()

	_, err := resilience.Open(resilience.Adapter("nope"), resilience.Options{})
	if !errors.Is(err, resilience.ErrUnknownAdapter) {
		t.Fatalf("Open(unknown) err = %v, want ErrUnknownAdapter", err)
	}

	var ue resilience.UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}

	if ue.Adapter != resilience.Adapter("nope") {
		t.Fatalf("carried adapter = %v, want nope", ue.Adapter)
	}
}

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := resilience.Register(a, nil); !errors.Is(err, resilience.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	ok := func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }

	if err := resilience.Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}

	err := resilience.Register(a, ok)
	if !errors.Is(err, resilience.ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}

	var de resilience.DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("err %T is not *DuplicateError", err)
	}

	if de.Adapter != a {
		t.Fatalf("carried adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	sentinel := errors.New("boom")

	if err := resilience.Register(a, func(resilience.Options) (resilience.Manager, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	_, err := resilience.Open(a, resilience.Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "resilience: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "resilience: open")
	}
}

func TestOpen_success_returnsManager(t *testing.T) {
	t.Parallel()

	a := freshAdapter()
	if err := resilience.Register(a, func(resilience.Options) (resilience.Manager, error) { return stubManager{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	m, err := resilience.Open(a, resilience.Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	g, err := m.Guard("dep")
	if err != nil {
		t.Fatalf("Guard err = %v", err)
	}

	if err := g.Execute(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Execute err = %v", err)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestDo_returnsValue(t *testing.T) {
	t.Parallel()

	g := &stubGuard{name: "dep"}

	got, err := resilience.Do(t.Context(), g, func(context.Context) (int, error) { return 42, nil })
	if err != nil {
		t.Fatalf("Do err = %v", err)
	}

	if got != 42 {
		t.Fatalf("Do = %d, want 42", got)
	}
}

func TestDo_propagatesError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	g := &stubGuard{name: "dep", err: sentinel}

	got, err := resilience.Do(t.Context(), g, func(context.Context) (int, error) { return 7, nil })
	if !errors.Is(err, sentinel) {
		t.Fatalf("Do err = %v, want sentinel", err)
	}

	if got != 0 {
		t.Fatalf("Do value = %d, want zero on error", got)
	}
}
