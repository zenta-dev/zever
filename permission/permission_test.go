package permission

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

var freshSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(freshSeq.Add(1)))
}

type stubChecker struct{}

func (stubChecker) Can(_ context.Context, _ Subject, _ string, _ Resource) (Decision, error) {
	return Decision{Allowed: true, Reason: "allow"}, nil
}

func TestRegister_nil_factory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Checker, error) { return stubChecker{}, nil }
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

func TestOpen_unknown_adapter(t *testing.T) {
	a := Adapter(9999)
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

func TestOpen_factory_error_wrapped(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (Checker, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "permission: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "permission: open")
	}
}

func TestOpen_success(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Checker, error) { return stubChecker{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	c, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	d, err := c.Can(context.Background(), Subject{}, "read", Resource{})
	if err != nil {
		t.Fatalf("Can err = %v", err)
	}
	if !d.Allowed {
		t.Fatalf("decision = %+v, want allowed", d)
	}
}
