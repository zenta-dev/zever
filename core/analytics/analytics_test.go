package analytics

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

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
