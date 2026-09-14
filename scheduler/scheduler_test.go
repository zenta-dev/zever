package scheduler

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/job"
)

func stubFactoryOpts(d *job.Dispatcher) Options {
	return Options{Dispatcher: d}
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
		return NewEmbedded(stubFactoryOpts(d))
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	err := Register(a, func(Options) (Scheduler, error) {
		return NewEmbedded(stubFactoryOpts(d))
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

	_, err := Open(Adapter(103), Options{})
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

	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v want wrapped sentinel", err)
	}
}

func TestOpenOk(t *testing.T) {
	t.Parallel()

	a := Adapter(105)
	d := &job.Dispatcher{Q: &stubQueue{}}

	if err := Register(a, func(Options) (Scheduler, error) {
		return NewEmbedded(stubFactoryOpts(d))
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
