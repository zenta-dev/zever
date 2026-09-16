package password_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/password"
)

type stubHasher struct{}

func (stubHasher) Hash(_ context.Context, p string) (string, error) {
	return "hash:" + p, nil
}

func (stubHasher) Verify(_ context.Context, _, p string) (bool, error) {
	return p == "secret", nil
}

func (stubHasher) NeedsRehash(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func stubFactory(_ password.Options) (password.Hasher, error) {
	return stubHasher{}, nil
}

func TestRegisterNil(t *testing.T) {
	t.Parallel()

	err := password.Register(password.Adapter(210), nil)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !errors.Is(err, password.ErrNilFactory) {
		t.Fatalf("errors.Is(%v, ErrNilFactory) = false", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()

	a := password.Adapter(211)

	if err := password.Register(a, stubFactory); err != nil {
		t.Fatalf("first Register() = %v, want nil", err)
	}

	err := password.Register(a, stubFactory)
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if !errors.Is(err, password.ErrDuplicate) {
		t.Fatalf("errors.Is(%v, ErrDuplicate) = false", err)
	}

	var dup *password.DuplicateError
	if !errors.As(err, &dup) {
		t.Fatalf("errors.As(%v) to *DuplicateError = false", err)
	}

	if dup.Adapter != a {
		t.Fatalf("Adapter = %v, want %v", dup.Adapter, a)
	}
}

func TestOpenUnknown(t *testing.T) {
	t.Parallel()

	h, err := password.Open(password.Adapter(998), password.Options{})
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if h != nil {
		t.Fatal("expected nil hasher")
	}

	if !errors.Is(err, password.ErrUnknownAdapter) {
		t.Fatalf("errors.Is(%v, ErrUnknownAdapter) = false", err)
	}

	var unk *password.UnknownAdapterError
	if !errors.As(err, &unk) {
		t.Fatalf("errors.As(%v) to *UnknownAdapterError = false", err)
	}
}

func TestOpenFactoryError(t *testing.T) {
	t.Parallel()

	a := password.Adapter(212)
	factoryErr := errors.New("boom")

	if err := password.Register(a, func(_ password.Options) (password.Hasher, error) {
		return nil, factoryErr
	}); err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}

	h, err := password.Open(a, password.Options{})
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	if h != nil {
		t.Fatal("expected nil hasher")
	}

	if !errors.Is(err, factoryErr) {
		t.Fatalf("errors.Is(%v, factoryErr) = false", err)
	}
}

func TestOpenSuccess(t *testing.T) {
	t.Parallel()

	a := password.Adapter(213)

	if err := password.Register(a, stubFactory); err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}

	ctx := context.Background()

	h, err := password.Open(a, password.Options{})
	if err != nil {
		t.Fatalf("Open() = %v, want nil", err)
	}

	if h == nil {
		t.Fatal("expected non-nil hasher")
	}

	hash, err := h.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	if hash != "hash:secret" {
		t.Fatalf("Hash() = %q, want %q", hash, "hash:secret")
	}

	ok, err := h.Verify(ctx, hash, "secret")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if !ok {
		t.Fatal("Verify() = false, want true")
	}

	ok, err = h.Verify(ctx, hash, "wrong")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false")
	}

	rehash, err := h.NeedsRehash(ctx, hash)
	if err != nil {
		t.Fatalf("NeedsRehash() = %v, want nil", err)
	}

	if rehash {
		t.Fatal("NeedsRehash() = true, want false")
	}
}

// TestConveniences is the only test that touches AdapterArgon2ID.
func TestConveniences(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	if _, err := password.Hash(ctx, "secret"); !errors.Is(err, password.ErrUnknownAdapter) {
		t.Fatalf("Hash() before register err = %v, want ErrUnknownAdapter", err)
	}

	if _, err := password.Verify(ctx, "hash:secret", "secret"); !errors.Is(err, password.ErrUnknownAdapter) {
		t.Fatalf("Verify() before register err = %v, want ErrUnknownAdapter", err)
	}

	if err := password.Register(password.AdapterArgon2ID, stubFactory); err != nil {
		t.Fatalf("Register() = %v, want nil", err)
	}

	hash, err := password.Hash(ctx, "secret")
	if err != nil {
		t.Fatalf("Hash() = %v, want nil", err)
	}

	if hash != "hash:secret" {
		t.Fatalf("Hash() = %q, want %q", hash, "hash:secret")
	}

	ok, err := password.Verify(ctx, hash, "secret")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if !ok {
		t.Fatal("Verify() = false, want true")
	}

	ok, err = password.Verify(ctx, hash, "wrong")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false")
	}
}

func TestConcurrentRegisterOpen(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			a := password.Adapter(100 + i)

			if err := password.Register(a, stubFactory); err != nil {
				t.Errorf("Register(%v) = %v, want nil", a, err)
				return
			}

			h, err := password.Open(a, password.Options{})
			if err != nil {
				t.Errorf("Open(%v) = %v, want nil", a, err)
				return
			}

			if h == nil {
				t.Errorf("Open(%v) returned nil hasher", a)
			}
		}(i)
	}

	wg.Wait()
}

func TestStubHasherEmptyInputs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := stubHasher{}

	cases := []struct {
		name string
		run  func() error
	}{
		{name: "hash empty", run: func() error { _, err := h.Hash(ctx, ""); return err }},
		{name: "verify empty", run: func() error { _, err := h.Verify(ctx, "", ""); return err }},
		{name: "needsrehash empty", run: func() error { _, err := h.NeedsRehash(ctx, ""); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.run(); err != nil {
				t.Fatalf("run() = %v, want nil", err)
			}
		})
	}

	ok, err := h.Verify(ctx, "", "")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false for empty password")
	}
}
