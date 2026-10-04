package firebase

import (
	"testing"

	"firebase.google.com/go/v4/remoteconfig"
)

// benchClient builds a hermetic client over the shared test template.
func benchClient(b *testing.B) *client {
	b.Helper()

	tpl, err := (&remoteconfig.Client{}).InitServerTemplate(map[string]any{}, testTemplate)
	if err != nil {
		b.Fatalf("InitServerTemplate: %v", err)
	}

	return &client{eval: tpl.Evaluate}
}

// BenchmarkBool measures the evaluate-and-coerce path for a bool flag.
func BenchmarkBool(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := c.Bool(ctx, "new_ui", false); err != nil {
			b.Fatalf("Bool: %v", err)
		}
	}
}

// BenchmarkString measures the evaluate-and-read path for a string flag.
func BenchmarkString(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := c.String(ctx, "welcome", ""); err != nil {
			b.Fatalf("String: %v", err)
		}
	}
}

// BenchmarkInt measures the evaluate-and-parse path for an int flag.
func BenchmarkInt(b *testing.B) {
	c := benchClient(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := c.Int(ctx, "retry_count", 0); err != nil {
			b.Fatalf("Int: %v", err)
		}
	}
}
