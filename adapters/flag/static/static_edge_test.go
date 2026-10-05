package static

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// TestEdgeKeyLengthBoundary checks the inclusive key-length limit and
// control-character rejection.
func TestEdgeKeyLengthBoundary(t *testing.T) {
	t.Parallel()

	f := openTestdata(t, false)
	ctx := t.Context()

	atLimit := strings.Repeat("k", 256)
	if _, err := f.Bool(ctx, atLimit, false); err != nil {
		t.Fatalf("Bool(256-byte key) = %v, want nil (missing -> fallback)", err)
	}

	overLimit := strings.Repeat("k", 257)
	if _, err := f.Bool(ctx, overLimit, false); !errors.Is(err, flag.ErrInvalidKey) {
		t.Fatalf("Bool(257-byte key) = %v, want ErrInvalidKey", err)
	}

	if _, err := f.Bool(ctx, "bad\tkey", false); !errors.Is(err, flag.ErrInvalidKey) {
		t.Fatalf("Bool(tab key) = %v, want ErrInvalidKey", err)
	}

	if _, err := f.String(ctx, "bad\x7fkey", ""); !errors.Is(err, flag.ErrInvalidKey) {
		t.Fatalf("Bool(DEL key) = %v, want ErrInvalidKey", err)
	}
}

// TestEdgeIntNegativeAndZero checks integer coercion at zero and negative
// values.
func TestEdgeIntNegativeAndZero(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"neg": -5, "zero": 0, "neg_float": -2.0}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	ctx := t.Context()

	if got, err := f.Int(ctx, "neg", 9); err != nil || got != -5 {
		t.Fatalf("Int(neg) = %d, %v; want -5, nil", got, err)
	}

	if got, err := f.Int(ctx, "zero", 9); err != nil || got != 0 {
		t.Fatalf("Int(zero) = %d, %v; want 0, nil", got, err)
	}

	if got, err := f.Int(ctx, "neg_float", 9); err != nil || got != -2 {
		t.Fatalf("Int(neg_float) = %d, %v; want -2 (trunc), nil", got, err)
	}
}

// TestEdgeIntPrecisionBoundary checks the 2^53 exactness boundary.
func TestEdgeIntPrecisionBoundary(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"exact": 9007199254740991, "imprecise": 9007199254740992}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	ctx := t.Context()

	if got, err := f.Int(ctx, "exact", 0); err != nil || got != 9007199254740991 {
		t.Fatalf("Int(2^53-1) = %d, %v; want exact value", got, err)
	}

	if _, err := f.Int(ctx, "imprecise", 0); err == nil {
		t.Fatal("Int(2^53) = nil error, want precision reject")
	}
}

// TestEdgeStringNegativeFloat checks negative float formatting.
func TestEdgeStringNegativeFloat(t *testing.T) {
	t.Parallel()

	p := writeTempFlags(t, `{"neg": -1.5}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	if got, err := f.String(t.Context(), "neg", "fb"); err != nil || got != "-1.5" {
		t.Fatalf("String(neg) = %q, %v; want -1.5, nil", got, err)
	}
}

// TestEdgeBoolNumericMismatch checks that a numeric value is not a bool.
func TestEdgeBoolNumericMismatch(t *testing.T) {
	t.Parallel()

	d := &driver{flags: map[string]any{"n": float64(1)}}
	if _, err := d.Bool(t.Context(), "n", false); err == nil {
		t.Fatal("Bool(number) = nil error, want mismatch")
	}
}

// TestEdgeJSONStructRoundtrip checks decoding a stored object into a struct.
func TestEdgeJSONStructRoundtrip(t *testing.T) {
	t.Parallel()

	f := openTestdata(t, false)

	var out struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}

	if err := f.JSON(t.Context(), "config", &out, nil); err != nil {
		t.Fatalf("JSON(config) error = %v", err)
	}

	if out.Host != "db.local" || out.Port != 5432 {
		t.Fatalf("out = %+v, want {db.local 5432}", out)
	}
}

// TestEdgeParseStrictBoolTable checks the accepted bool spellings.
func TestEdgeParseStrictBoolTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
		err  bool
	}{
		{"true", true, false},
		{"TRUE", true, false},
		{"False", false, false},
		{"false", false, false},
		{"", false, true},
		{"1", false, true},
		{"yes", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()

			got, err := parseStrictBool(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("parseStrictBool(%q) = %v, nil; want error", tc.in, got)
				}

				return
			}

			if err != nil || got != tc.want {
				t.Fatalf("parseStrictBool(%q) = %v, %v; want %v, nil", tc.in, got, err, tc.want)
			}
		})
	}
}

// TestEdgeConcurrentReads exercises the goroutine-safe driver under
// concurrent typed reads.
func TestEdgeConcurrentReads(t *testing.T) {
	t.Parallel()

	f := openTestdata(t, false)
	ctx := t.Context()

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 50 {
				if _, err := f.Bool(ctx, "debug", false); err != nil {
					t.Errorf("Bool() error = %v", err)
					return
				}

				if _, err := f.String(ctx, "env", ""); err != nil {
					t.Errorf("String() error = %v", err)
					return
				}

				if _, err := f.Int(ctx, "max_retries", 0); err != nil {
					t.Errorf("Int() error = %v", err)
					return
				}

				var out map[string]any
				if err := f.JSON(ctx, "config", &out, nil); err != nil {
					t.Errorf("JSON() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
