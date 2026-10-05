package flag

import (
	"testing"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := Register(freshAdapter(), func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

// BenchmarkOpen measures validated registry lookup plus the factory call.
func BenchmarkOpen(b *testing.B) {
	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (Flag, error) { return &stubFlag{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		f, err := Open(adapter, Options{})
		if err != nil {
			b.Fatalf("Open err = %v", err)
		}

		_ = f.Close()
	}
}

// BenchmarkValidateKey measures flag key shape validation.
func BenchmarkValidateKey(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := ValidateKey("feature.new-ui"); err != nil {
			b.Fatalf("ValidateKey err = %v", err)
		}
	}
}

// BenchmarkGetJSON measures typed JSON decoding through the Flag contract.
func BenchmarkGetJSON(b *testing.B) {
	f := &jsonStubFlag{values: map[string]any{"app": `{"port":8080}`}}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := GetJSON(ctx, f, "app", jsonConfig{}); err != nil {
			b.Fatalf("GetJSON err = %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures option validation.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

// BenchmarkEvalContextRoundTrip measures attaching and reading an EvalContext.
func BenchmarkEvalContextRoundTrip(b *testing.B) {
	ec := EvalContext{RandomizationID: "user-123", Signals: map[string]any{"plan": "pro"}}
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		got, ok := EvalContextFrom(WithEvalContext(ctx, ec))
		if !ok || got.RandomizationID != ec.RandomizationID {
			b.Fatal("EvalContext round trip failed")
		}
	}
}

// BenchmarkAdapterString measures canonical adapter naming.
func BenchmarkAdapterString(b *testing.B) {
	a := Firebase

	b.ReportAllocs()

	for b.Loop() {
		_ = a.String()
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := ParseAdapter("firebase"); err != nil {
			b.Fatalf("ParseAdapter err = %v", err)
		}
	}
}
