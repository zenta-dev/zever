package document

import (
	"context"
)

type stubDocument struct {
	out []byte
}

func (s *stubDocument) Render(_ context.Context, _ []byte, _ OutputFormat) ([]byte, error) {
	return s.out, nil
}

func (s *stubDocument) Close() error {
	return nil
}
