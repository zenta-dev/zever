package db

import (
	"context"
	"sync/atomic"
)

var dbAdapterSeq atomic.Int64

type stubRows struct{}

func (r *stubRows) Next() bool                 { return false }
func (r *stubRows) Scan(_ ...any) error        { return nil }
func (r *stubRows) Close() error               { return nil }
func (r *stubRows) Columns() ([]string, error) { return []string{"id"}, nil }
func (r *stubRows) Err() error                 { return nil }

type stubDB struct {
	dialect string
}

func (s *stubDB) Query(_ context.Context, _ string, _ ...any) (Rows, error) {
	return &stubRows{}, nil
}

func (s *stubDB) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 1, nil
}

func (s *stubDB) Ping(_ context.Context) error { return nil }

func (s *stubDB) Close(_ context.Context) error { return nil }

func (s *stubDB) Dialect() string { return s.dialect }
