package migrate

import "testing"

// BenchmarkChecksumOf measures the sha256 dedup-key computation paid once
// per DDL statement during migration planning and recording.
func BenchmarkChecksumOf(b *testing.B) {
	stmt := `CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, email TEXT NOT NULL UNIQUE, created_at TEXT NOT NULL)`

	b.ReportAllocs()

	for b.Loop() {
		_ = ChecksumOf(stmt)
	}
}
