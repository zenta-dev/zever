package search

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

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
