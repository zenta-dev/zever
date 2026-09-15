package media

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

func TestRegister_nilFactory_returnsNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	ok := func(Options) (Media, error) { return &stubMedia{}, nil }
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

	if err := Register(a, func(Options) (Media, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}

	if !strings.Contains(err.Error(), "media: open") {
		t.Fatalf("Open err %q missing %q", err.Error(), "media: open")
	}

	if got != nil {
		t.Fatalf("Open error value = %v, want nil", got)
	}
}

func TestOpen_success_smoke(t *testing.T) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Media, error) { return &stubMedia{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	ctx := context.Background()

	if _, err := got.Upload(ctx, "p/a.jpg", []byte("data"), UploadOptions{ContentType: "image/jpeg"}); err != nil {
		t.Fatalf("Upload err = %v", err)
	}

	if _, err := got.Download(ctx, "id"); err != nil {
		t.Fatalf("Download err = %v", err)
	}

	if _, err := got.DownloadRange(ctx, "id", 0, 0); err != nil {
		t.Fatalf("DownloadRange err = %v", err)
	}

	if err := got.Delete(ctx, "id"); err != nil {
		t.Fatalf("Delete err = %v", err)
	}

	if _, err := got.Stat(ctx, "id"); err != nil {
		t.Fatalf("Stat err = %v", err)
	}

	if _, err := got.Probe(ctx, "id"); err != nil {
		t.Fatalf("Probe err = %v", err)
	}

	if _, err := got.Transform(ctx, "id", TransformOps{}); err != nil {
		t.Fatalf("Transform err = %v", err)
	}

	if err := got.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_invalidOptions_validatedFirst(t *testing.T) {
	a := freshAdapter()

	got, err := Open(a, Options{MaxDownloadBytes: -1})
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
