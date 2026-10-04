package idempotency

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestRegisterShared_nilFactory_returnsErrNilFactory(t *testing.T) {
	a := freshAdapter()
	if err := RegisterShared(a, nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("RegisterShared nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegisterShared_duplicate_returnsDuplicateError(t *testing.T) {
	a := freshAdapter()
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		t.Fatalf("first RegisterShared err = %v", err)
	}
	err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return &stubStore{}, nil })
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second RegisterShared err = %v, want ErrDuplicate", err)
	}
}

func TestOpenShared_unknownAdapter_returnsUnknownError(t *testing.T) {
	a := freshAdapter()
	_, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("OpenShared unknown err = %v, want ErrUnknownAdapter", err)
	}
}

func TestOpenShared_factoryError_wrappedWithAdapter(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return nil, sentinel }); err != nil {
		t.Fatalf("RegisterShared err = %v", err)
	}
	_, err := OpenShared(a, nil, Options{})
	if !errors.Is(err, sentinel) {
		t.Fatalf("OpenShared err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "idempotency: open shared") {
		t.Fatalf("OpenShared err %q missing %q", err.Error(), "idempotency: open shared")
	}
}

func TestOpenShared_invalidOptions_validatedBeforeLookup(t *testing.T) {
	a := freshAdapter()
	_, err := OpenShared(a, nil, Options{TTL: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("OpenShared invalid opts err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpenShared_success_returnsStore(t *testing.T) {
	a := freshAdapter()
	want := &stubStore{}
	if err := RegisterShared(a, func(coredb.DB, Options) (Store, error) { return want, nil }); err != nil {
		t.Fatalf("RegisterShared err = %v", err)
	}
	got, err := OpenShared(a, nil, Options{})
	if err != nil {
		t.Fatalf("OpenShared err = %v", err)
	}
	if got != want {
		t.Fatal("OpenShared did not return factory store")
	}
}

func TestOpen_invalidOptions_failsBeforeLookup(t *testing.T) {
	a := freshAdapter()
	_, err := Open(a, Options{TTL: -time.Second})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Open invalid opts err = %v, want ErrInvalidOptions", err)
	}
}

func TestValidateKey_multibyteByteLengthBoundary(t *testing.T) {
	t.Parallel()
	multibyte := strings.Repeat("é", 127) + "a"
	if len(multibyte) != MaxKeyLen {
		t.Fatalf("test setup: len = %d, want %d", len(multibyte), MaxKeyLen)
	}
	if err := ValidateKey(multibyte); err != nil {
		t.Fatalf("ValidateKey(255 bytes multibyte) err = %v, want nil", err)
	}

	overBytes := strings.Repeat("é", 128)
	if len(overBytes) != MaxKeyLen+1 {
		t.Fatalf("test setup: len = %d, want %d", len(overBytes), MaxKeyLen+1)
	}
	if err := ValidateKey(overBytes); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("ValidateKey(256 bytes multibyte) err = %v, want ErrInvalidKey", err)
	}
}

func TestValidateKey_invalidUTF8_accepted(t *testing.T) {
	t.Parallel()
	if err := ValidateKey("a\xffb"); err != nil {
		t.Fatalf("ValidateKey(invalid utf8) err = %v, want nil (RuneError skipped)", err)
	}
}

func TestRegister_concurrent_uniqueAdapters_allSucceed(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup
	errs := make([]error, 20)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = Register(freshAdapter(), func(Options) (Store, error) { return &stubStore{}, nil })
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}
