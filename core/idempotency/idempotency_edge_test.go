package idempotency

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
)

func TestIdempotencyEdge_KeyBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "one valid", key: "a"},
		{name: "max valid", key: strings.Repeat("a", MaxKeyLen)},
		{name: "empty invalid", key: "", wantErr: true},
		{name: "too long invalid", key: strings.Repeat("a", MaxKeyLen+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateKey(tt.key)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateKey() err = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr {
				if !errors.Is(err, ErrInvalidKey) {
					t.Fatalf("errors.Is(err, ErrInvalidKey) = false (err = %v)", err)
				}

				got, ok := errors.AsType[InvalidKeyError](err)
				if !ok || got.KeyLen != len(tt.key) {
					t.Fatalf("errors.AsType[InvalidKeyError] = %+v, %v; want KeyLen %d", got, ok, len(tt.key))
				}
			}
		})
	}
}

func TestIdempotencyEdge_TTLDefault(t *testing.T) {
	t.Parallel()

	if got := (Options{}).ttl(); got != DefaultTTL {
		t.Fatalf("zero Options ttl() = %v, want %v", got, DefaultTTL)
	}
}

func TestIdempotencyEdge_OptionsValidateBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "zero valid", opts: Options{}},
		{name: "ttl zero valid", opts: Options{TTL: 0}},
		{name: "negative max entries valid", opts: Options{MaxEntries: -1}},
		{name: "ttl negative invalid", opts: Options{TTL: -time.Second}, wantErr: true},
		{name: "bad redis addr invalid", opts: Options{Redis: RedisOptions{Addr: "not-an-addr"}}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.opts.Validate() != nil; got != tt.wantErr {
				t.Fatalf("Validate() error present = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestIdempotencyEdge_RegisterConcurrentUnique(t *testing.T) {
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

func TestIdempotencyEdge_OpenConcurrent(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (Store, error) { return &stubStore{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			store, err := Open(adapter, Options{})
			if err != nil {
				t.Errorf("Open err = %v", err)

				return
			}

			_ = store.Close()
		}()
	}

	wg.Wait()
}

func TestIdempotencyEdge_OpenSharedNilConn(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	want := &stubStore{}

	if err := RegisterShared(adapter, func(conn coredb.DB, _ Options) (Store, error) {
		if conn != nil {
			t.Errorf("conn = %v, want nil", conn)
		}

		return want, nil
	}); err != nil {
		t.Fatalf("RegisterShared err = %v", err)
	}

	got, err := OpenShared(adapter, nil, Options{})
	if err != nil {
		t.Fatalf("OpenShared err = %v", err)
	}

	if got != want {
		t.Fatal("OpenShared did not return the factory store")
	}
}
