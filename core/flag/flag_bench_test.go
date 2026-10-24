package flag

import (
	"testing"
)

// BenchmarkValidateKey measures flag key validation.
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
