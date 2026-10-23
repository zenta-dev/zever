package secrets_test

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// benchSecretsAdapter registers a stub secrets backend once and returns its
// adapter so Open can be measured without the one-shot registration cost.
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
