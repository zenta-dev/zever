package dbconn

import "testing"

// BenchmarkValidateTableName measures validating a well-formed table name.
func BenchmarkValidateTableName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := ValidateTableName("queue_messages"); err != nil {
			b.Fatalf("ValidateTableName() error = %v", err)
		}
	}
}

// BenchmarkIsPostgresDSN measures classifying a postgres DSN.
func BenchmarkIsPostgresDSN(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = IsPostgresDSN("postgres://user:pass@localhost:5432/zever?sslmode=disable")
	}
}

// BenchmarkSplitDSN measures mapping a DSN onto pool options.
func BenchmarkSplitDSN(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = SplitDSN("postgres://user:pass@localhost:5432/zever?sslmode=disable")
	}
}
