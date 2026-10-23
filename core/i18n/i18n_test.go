package i18n

import (
	"context"
	"sync/atomic"
)

var freshSeq atomic.Int64

type stubI18n struct{}

func (s *stubI18n) Translate(_ context.Context, _, _ string, _ map[string]string) (string, error) {
	return "", nil
}

func (s *stubI18n) Locales(_ context.Context) ([]string, error) { return nil, nil }

func (s *stubI18n) Close() error { return nil }
