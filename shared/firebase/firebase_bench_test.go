package firebase

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkValidateServiceAccountPath measures the pure path-policy check.
func BenchmarkValidateServiceAccountPath(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := ValidateServiceAccountPath("config/service-account.json"); err != nil {
			b.Fatalf("ValidateServiceAccountPath() error = %v", err)
		}
	}
}

// BenchmarkCredentialsValidate measures full credential validation, including
// reading and JSON-parsing the service account file.
func BenchmarkCredentialsValidate(b *testing.B) {
	path := filepath.Join(b.TempDir(), "sa.json")
	if err := os.WriteFile(path, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		b.Fatalf("write key = %v", err)
	}

	c := Credentials{ProjectID: "p", ServiceAccount: path}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if err := c.Validate(); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
