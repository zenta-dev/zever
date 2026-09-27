package idempotency

import (
	"context"
)

type stubStore struct{}

func (s *stubStore) Begin(_ context.Context, _ string, _ BeginOptions) (Outcome, error) {
	return Outcome{}, nil
}

func (s *stubStore) Complete(_ context.Context, _ string, _, _ []byte) error {
	return nil
}

func (s *stubStore) Forget(_ context.Context, _ string) error { return nil }

func (s *stubStore) Close() error { return nil }
