package flag

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

type stubFlag struct{}

func (s *stubFlag) Bool(_ context.Context, _ string, fallback bool) (bool, error) {
	return fallback, nil
}

func (s *stubFlag) String(_ context.Context, _ string, fallback string) (string, error) {
	return fallback, nil
}

func (s *stubFlag) Int(_ context.Context, _ string, fallback int) (int, error) {
	return fallback, nil
}

func (s *stubFlag) JSON(_ context.Context, _ string, _ any, _ any) error { return nil }

func (s *stubFlag) Close() error { return nil }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Flag, error) { return &stubFlag{}, nil }
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
	if err := Register(a, func(Options) (Flag, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "flag: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "flag: open")
	}
}

func TestOpen_success_returnsFlag(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	f, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestWithEvalContext_roundtrip_present(t *testing.T) {
	t.Parallel()
	ec := EvalContext{RandomizationID: "user-123", Signals: map[string]any{"plan": "pro"}}
	ctx := WithEvalContext(t.Context(), ec)
	got, ok := EvalContextFrom(ctx)
	if !ok {
		t.Fatal("EvalContextFrom = false, want true")
	}
	if got.RandomizationID != ec.RandomizationID {
		t.Errorf("RandomizationID = %q want %q", got.RandomizationID, ec.RandomizationID)
	}
	if got.Signals["plan"] != "pro" {
		t.Errorf("Signals = %v want plan=pro", got.Signals)
	}
}

func TestEvalContextFrom_missing_returnsFalse(t *testing.T) {
	t.Parallel()
	if _, ok := EvalContextFrom(t.Context()); ok {
		t.Error("EvalContextFrom = true, want false")
	}
}
