package i18n

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

type stubI18n struct{}

func (s *stubI18n) Translate(_ context.Context, _, _ string, _ map[string]string) (string, error) {
	return "", nil
}

func (s *stubI18n) Locales(_ context.Context) ([]string, error) { return nil, nil }

func (s *stubI18n) Close() error { return nil }

func TestRegister_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (I18n, error) { return &stubI18n{}, nil }
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

func TestOpen_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := Register(a, func(Options) (I18n, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "i18n: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "i18n: open")
	}
}

func TestOpen_success_returnsI18n(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (I18n, error) { return &stubI18n{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	n, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}
