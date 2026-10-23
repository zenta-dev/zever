package static

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// stubFlag is a no-op flag.Flag used to isolate registry benchmarks.
type stubFlag struct{}

func (stubFlag) Bool(context.Context, string, bool) (bool, error)       { return false, nil }
func (stubFlag) String(context.Context, string, string) (string, error) { return "", nil }
func (stubFlag) Int(context.Context, string, int) (int, error)          { return 0, nil }
func (stubFlag) JSON(context.Context, string, any, any) error           { return nil }
func (stubFlag) Close() error                                           { return nil }

// benchFlag opens the checked-in flags fixture for benchmarks.
func benchFlag(b *testing.B) flag.Flag {
	b.Helper()

	f, err := New(flag.Options{Static: flag.StaticOptions{Path: "testdata/flags.json"}})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = f.Close() })

	return f
}

// BenchmarkNew measures constructing an empty (default) flag set.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		f, err := New(flag.Options{})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = f.Close()
	}
}

// BenchmarkBool measures a map lookup plus boolean coercion.
func BenchmarkBool(b *testing.B) {
	f := benchFlag(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := f.Bool(ctx, "debug", false); err != nil {
			b.Fatalf("Bool() error = %v", err)
		}
	}
}

// BenchmarkString measures a map lookup plus string coercion.
func BenchmarkString(b *testing.B) {
	f := benchFlag(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := f.String(ctx, "env", ""); err != nil {
			b.Fatalf("String() error = %v", err)
		}
	}
}

// BenchmarkInt measures a map lookup plus integer coercion.
func BenchmarkInt(b *testing.B) {
	f := benchFlag(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := f.Int(ctx, "max_retries", 0); err != nil {
			b.Fatalf("Int() error = %v", err)
		}
	}
}

// BenchmarkJSON measures a map lookup plus JSON decoding.
func BenchmarkJSON(b *testing.B) {
	f := benchFlag(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out map[string]any

		if err := f.JSON(ctx, "config", &out, nil); err != nil {
			b.Fatalf("JSON() error = %v", err)
		}
	}
}

// BenchmarkParseStrictBool measures the strict bool parser.
func BenchmarkParseStrictBool(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := parseStrictBool("TRUE"); err != nil {
			b.Fatalf("parseStrictBool() error = %v", err)
		}
	}
}

// BenchmarkFloatToInt measures the exact-integer float coercion.
func BenchmarkFloatToInt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := floatToInt("k", 0, 1234); err != nil {
			b.Fatalf("floatToInt() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		f, openErr := New(flag.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := f.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
