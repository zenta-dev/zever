package postgres

import (
	"testing"
)

// benchQueries returns representative queries covering plain placeholders,
// string literals, dollar-quoted bodies, JSONB operators, and comments.
func benchQueries() []string {
	return []string{
		"SELECT * FROM users WHERE id = ? AND name = ?",
		"SELECT * FROM users WHERE id IN (?, ?, ?)",
		"SELECT '?' AS literal, id FROM users WHERE id = ?",
		"SELECT * FROM events WHERE data ? 'key' AND id = ?",
		"SELECT $func$ ? $func$ AS body, id FROM users WHERE id = ?",
		"SELECT * FROM users WHERE id = ? -- comment ?\n AND name = ?",
		"INSERT INTO t (a, b, c, d) VALUES (?, ?, ?, ?)",
	}
}

// BenchmarkReplacePlaceholders measures cached ?-to-$N rewriting across a
// spread of query shapes.
func BenchmarkReplacePlaceholders(b *testing.B) {
	queries := benchQueries()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = replacePlaceholders(queries[i%len(queries)])
	}
}

// BenchmarkReplacePlaceholdersUncached measures the same rewriting without the
// result cache, isolating the scanner cost.
func BenchmarkReplacePlaceholdersUncached(b *testing.B) {
	queries := benchQueries()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = replacePlaceholdersUncached(queries[i%len(queries)])
	}
}

// BenchmarkReplacePlaceholdersParallel measures cache-hit throughput under
// concurrent load.
func BenchmarkReplacePlaceholdersParallel(b *testing.B) {
	const query = "SELECT * FROM users WHERE id = ? AND name = ?"

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = replacePlaceholders(query)
		}
	})
}
