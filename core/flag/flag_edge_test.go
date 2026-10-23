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

func TestFlagEdge_GetJSONNilFallback(t *testing.T) {
	t.Parallel()

	f := &jsonStubFlag{values: map[string]any{}}

	got, err := GetJSON(t.Context(), f, "missing", []string(nil))
	if err != nil {
		t.Fatalf("GetJSON err = %v, want nil", err)
	}

	// shared/codec (encoding/json/v2) renders a nil slice as [] rather than
	// null, so the missing-key marshal/unmarshal round-trip yields an empty
	// non-nil slice instead of the v1 nil. The fallback elements are
	// preserved; only the slice's nil-ness is lost.
	if got == nil || len(got) != 0 {
		t.Fatalf("GetJSON = %v, want empty fallback", got)
	}
}

func TestFlagEdge_EvalContextMissing(t *testing.T) {
	t.Parallel()

	if _, ok := EvalContextFrom(t.Context()); ok {
		t.Fatal("EvalContextFrom = true, want false without WithEvalContext")
	}
}

func TestFlagEdge_ConcurrentFlagCalls(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	f := &stubFlag{}
	jf := &jsonStubFlag{values: map[string]any{"app": `{"port":8080}`}}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := f.Bool(ctx, "x", true); err != nil {
				t.Errorf("Bool err = %v", err)

				return
			}

			if _, err := f.String(ctx, "x", "s"); err != nil {
				t.Errorf("String err = %v", err)

				return
			}

			if _, err := f.Int(ctx, "x", 1); err != nil {
				t.Errorf("Int err = %v", err)

				return
			}

			var out jsonConfig
			if err := jf.JSON(ctx, "app", &out, jsonConfig{}); err != nil {
				t.Errorf("JSON err = %v", err)
			}
		}()
	}

	wg.Wait()
}
