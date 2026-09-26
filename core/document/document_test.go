package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var testSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", 1000+int(testSeq.Add(1))))
}

type stubDocument struct {
	out []byte
}

func (s *stubDocument) Render(_ context.Context, _ []byte, _ OutputFormat) ([]byte, error) {
	return s.out, nil
}

func (s *stubDocument) Close() error {
	return nil
}

func TestRegister_nilFactory_returnsNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Document, error) { return &stubDocument{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}

	if err := Register(a, ok); !errors.Is(err, ErrDuplicateAdapter) {
		t.Fatalf("second Register err = %v, want ErrDuplicateAdapter", err)
	}

	var de *DuplicateAdapterError

	if err := Register(a, ok); !errors.As(err, &de) {
		t.Fatalf("dup err %T is not *DuplicateAdapterError", err)
	} else if de.Adapter != a {
		t.Fatalf("dup carried adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknownAdapter_returnsUnknownAndNil(t *testing.T) {
	a := Adapter("test-9999")

	got, err := Open(a, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}

	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("err %T is not *UnknownAdapterError", err)
	}

	if ue.Adapter != a {
		t.Fatalf("carried adapter = %v, want %v", ue.Adapter, a)
	}

	if got != nil {
		t.Fatalf("Open unknown value = %v, want nil", got)
	}
}

func TestOpen_factoryError_wrappedWithPrefixAndNil(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")

	if err := Register(a, func(Options) (Document, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "document: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "document: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	want := []byte("%PDF-1.4 stub")
	if err := Register(a, func(Options) (Document, error) { return &stubDocument{out: want}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := t.Context()

	out, err := got.Render(ctx, []byte("source"), FormatPDF)
	if err != nil {
		t.Fatalf("Render err = %v", err)
	}

	if string(out) != string(want) {
		t.Fatalf("Render = %q, want %q", out, want)
	}

	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Document, error) { return &stubDocument{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{Timeout: -1})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}

	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}
