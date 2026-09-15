package analytics

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

var testSeq atomic.Int64

func freshAdapter() Adapter {
	return Adapter(1000 + int(testSeq.Add(1)))
}

type stubAnalytics struct {
	trackErr error
	closed   bool
}

func (s *stubAnalytics) Track(_ context.Context, _ string, _ map[string]any) error {
	return s.trackErr
}

func (s *stubAnalytics) Identify(_ context.Context, _ string, _ map[string]any) error {
	return nil
}

func (s *stubAnalytics) Group(_ context.Context, _ string, _ string, _ map[string]any) error {
	return nil
}

func (s *stubAnalytics) Close() error {
	s.closed = true

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
	ok := func(Options) (Analytics, error) { return &stubAnalytics{}, nil }
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
	a := Adapter(9999)
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
	if err := Register(a, func(Options) (Analytics, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "analytics: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "analytics: open")
	}
	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_tracksAndCloses(t *testing.T) {
	a := freshAdapter()
	stub := &stubAnalytics{}
	if err := Register(a, func(Options) (Analytics, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := got.Track(context.Background(), "evt", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("Track err = %v", err)
	}
	if err := got.Identify(context.Background(), "u1", nil); err != nil {
		t.Fatalf("Identify err = %v", err)
	}
	if err := got.Group(context.Background(), "u1", "g1", nil); err != nil {
		t.Fatalf("Group err = %v", err)
	}
	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Analytics, error) { return &stubAnalytics{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{MaxProperties: -1})
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
