package search

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

type stubSearch struct{}

func (s *stubSearch) Index(_ context.Context, _ Document) error {
	return nil
}

func (s *stubSearch) IndexBatch(_ context.Context, _ []Document) error {
	return nil
}

func (s *stubSearch) Delete(_ context.Context, _ string) error {
	return nil
}

func (s *stubSearch) Search(_ context.Context, _ string, _ QueryOptions) (Result, error) {
	return Result{Hits: []Hit{{ID: "doc-1", Score: 1.0}}, Total: 1}, nil
}

func (s *stubSearch) Close() error {
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
	ok := func(Options) (Search, error) { return &stubSearch{}, nil }
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

	if err := Register(a, func(Options) (Search, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "search: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "search: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Search, error) { return &stubSearch{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := t.Context()

	if err = got.Index(ctx, Document{ID: "doc-1", Index: "main", Content: "hello"}); err != nil {
		t.Fatalf("Index err = %v", err)
	}

	res, err := got.Search(ctx, "hello", QueryOptions{Limit: 10, Filters: map[string]string{"index": "main"}})
	if err != nil {
		t.Fatalf("Search err = %v", err)
	}

	if res.Total != 1 {
		t.Fatalf("Total = %d, want 1", res.Total)
	}

	if len(res.Hits) != 1 || res.Hits[0].ID != "doc-1" {
		t.Fatalf("Hits = %+v, want one hit doc-1", res.Hits)
	}

	if err = got.Delete(ctx, "doc-1"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	if err = got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_propagatesInvalidOptions(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Search, error) { return &stubSearch{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{Host: "localhost:7700"})
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
