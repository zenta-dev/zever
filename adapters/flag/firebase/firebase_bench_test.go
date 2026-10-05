package firebase

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"firebase.google.com/go/v4/remoteconfig"

	"github.com/zenta-dev/zever/core/flag"
)

var benchAdapterSeq atomic.Int64

// benchFreshAdapter returns an adapter name unique to this benchmark run so
// Register never collides with a previously registered factory.
func benchFreshAdapter() flag.Adapter {
	return flag.Adapter(fmt.Sprintf("bench-%d", benchAdapterSeq.Add(1)))
}

// stubFlag is a no-op flag.Flag used to isolate registry benchmarks.
type stubFlag struct{}

func (stubFlag) Bool(context.Context, string, bool) (bool, error)       { return false, nil }
func (stubFlag) String(context.Context, string, string) (string, error) { return "", nil }
func (stubFlag) Int(context.Context, string, int) (int, error)          { return 0, nil }
func (stubFlag) JSON(context.Context, string, any, any) error           { return nil }
func (stubFlag) Close() error                                           { return nil }

// benchClient builds a client over the hermetic test template.
func benchClient(b *testing.B) *client {
	b.Helper()

	tpl, err := (&remoteconfig.Client{}).InitServerTemplate(map[string]any{}, testTemplate)
	if err != nil {
		b.Fatalf("InitServerTemplate() error = %v", err)
	}

	return &client{eval: tpl.Evaluate}
}

// BenchmarkBool measures evaluate plus boolean coercion.
func BenchmarkBool(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.Bool(ctx, "new_ui", false); err != nil {
			b.Fatalf("Bool() error = %v", err)
		}
	}
}

// BenchmarkString measures evaluate plus string lookup.
func BenchmarkString(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.String(ctx, "welcome", ""); err != nil {
			b.Fatalf("String() error = %v", err)
		}
	}
}

// BenchmarkInt measures evaluate plus integer parsing.
func BenchmarkInt(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.Int(ctx, "retry_count", 0); err != nil {
			b.Fatalf("Int() error = %v", err)
		}
	}
}

// BenchmarkJSON measures evaluate plus JSON decoding.
func BenchmarkJSON(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var out map[string]any

		if err := c.JSON(ctx, "cfg", &out, nil); err != nil {
			b.Fatalf("JSON() error = %v", err)
		}
	}
}

// BenchmarkParseFirebaseInt measures the integer coercion helper.
func BenchmarkParseFirebaseInt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := parseFirebaseInt("42"); err != nil {
			b.Fatalf("parseFirebaseInt() error = %v", err)
		}
	}
}

// BenchmarkEvalContextMap measures building the evaluation signal map.
func BenchmarkEvalContextMap(b *testing.B) {
	ctx := flag.WithEvalContext(b.Context(), flag.EvalContext{
		RandomizationID: "user-1",
		Signals:         map[string]any{"plan": "pro"},
	})

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = evalContextMap(ctx)
	}
}

// BenchmarkRegister measures registering a factory into the flag registry.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		err := flag.Register(benchFreshAdapter(), func(flag.Options) (flag.Flag, error) {
			return stubFlag{}, nil
		})
		if err != nil {
			b.Fatalf("Register() error = %v", err)
		}
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()

	err := flag.Register(a, func(flag.Options) (flag.Flag, error) { return stubFlag{}, nil })
	if err != nil {
		b.Fatalf("Register() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		f, openErr := flag.Open(a, flag.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := f.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
