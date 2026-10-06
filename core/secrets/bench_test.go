package secrets_test

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// benchSecretsAdapter registers a stub secrets backend once and returns its
// adapter so Open can be measured without the one-shot registration cost.
func benchSecretsAdapter(b *testing.B) secrets.Adapter {
	b.Helper()

	a := freshSecretsAdapter()
	if err := secrets.Register(a, func(secrets.Options) (secrets.Secrets, error) { return stubSecrets{}, nil }); err != nil {
		b.Fatalf("Register(%v) error = %v", a, err)
	}

	return a
}

func BenchmarkOpen(b *testing.B) {
	a := benchSecretsAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := secrets.Open(a, secrets.Options{}); err != nil {
			b.Fatalf("Open(%v) error = %v", a, err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchSecretsAdapter(b)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := secrets.Open(a, secrets.Options{}); err != nil {
				b.Errorf("Open(%v) error = %v", a, err)
				return
			}
		}
	})
}

func BenchmarkValidateName(b *testing.B) {
	name := "db.password_2024"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := secrets.ValidateName(name); err != nil {
			b.Fatalf("ValidateName(%q) error = %v", name, err)
		}
	}
}

func BenchmarkValidateNameLong(b *testing.B) {
	name := strings.Repeat("a", 1024)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := secrets.ValidateName(name); err != nil {
			b.Fatalf("ValidateName(long) error = %v", err)
		}
	}
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := secrets.Options{Addr: "https://vault:8200", Prefix: "app/"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
