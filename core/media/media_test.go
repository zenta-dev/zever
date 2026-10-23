package media

import (
	"context"
	"sync/atomic"
)

var testSeq atomic.Int64

type stubMedia struct{}

func (s *stubMedia) Upload(_ context.Context, _ string, _ []byte, _ UploadOptions) (Asset, error) {
	return Asset{}, nil
}

func (s *stubMedia) Download(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

func (s *stubMedia) DownloadRange(_ context.Context, _ string, _, _ int64) ([]byte, error) {
	return nil, nil
}

func (s *stubMedia) Delete(_ context.Context, _ string) error {
	return nil
}

func (s *stubMedia) Stat(_ context.Context, _ string) (Info, error) {
	return Info{}, nil
}

func (s *stubMedia) Probe(_ context.Context, _ string) (Probe, error) {
	return Probe{}, nil
}

func (s *stubMedia) Transform(_ context.Context, _ string, _ TransformOps) (string, error) {
	return "", nil
}

func (s *stubMedia) Close() error {
	return nil
}
