package flag

import (
	"errors"
	"strings"
	"sync"
	"testing"
)

func TestFlagEdge_ValidateKeyBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{name: "one valid", key: "a"},
		{name: "max valid", key: strings.Repeat("a", maxKeyLen)},
		{name: "empty invalid", key: "", wantErr: true},
		{name: "too long invalid", key: strings.Repeat("a", maxKeyLen+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateKey(tt.key)
			if tt.wantErr != (err != nil) {
				t.Fatalf("ValidateKey() err = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && !errors.Is(err, ErrInvalidKey) {
				t.Fatalf("errors.Is(err, ErrInvalidKey) = false (err = %v)", err)
			}
		})
	}
}

func TestFlagEdge_OpenConcurrent(t *testing.T) {
	t.Parallel()

	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := Open(adapter, Options{}); err != nil {
				t.Errorf("Open err = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestFlagEdge_RegisterConcurrentUnique(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range errs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = Register(freshAdapter(), func(Options) (Flag, error) { return &stubFlag{}, nil })
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}

func TestFlagEdge_GetJSONNilFallback(t *testing.T) {
	t.Parallel()

	f := &jsonStubFlag{values: map[string]any{}}

	got, err := GetJSON(t.Context(), f, "missing", []string(nil))
	if err != nil {
		t.Fatalf("GetJSON err = %v, want nil", err)
	}

	if got != nil {
		t.Fatalf("GetJSON = %v, want nil fallback", got)
	}
}

func TestFlagEdge_EvalContextMissing(t *testing.T) {
	t.Parallel()

	if _, ok := EvalContextFrom(t.Context()); ok {
		t.Fatal("EvalContextFrom = true, want false without WithEvalContext")
	}
}
