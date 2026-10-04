package sqlite

import "testing"

// BenchmarkPlaceholder measures the per-bound-value placeholder path.
func BenchmarkPlaceholder(b *testing.B) {
	d := New()

	b.ReportAllocs()
	for b.Loop() {
		_ = d.Placeholder(7)
	}
}

// BenchmarkQuoteIdent measures quoting a plain identifier.
func BenchmarkQuoteIdent(b *testing.B) {
	d := New()

	b.ReportAllocs()
	for b.Loop() {
		_ = d.QuoteIdent("widgets")
	}
}

// BenchmarkQuoteIdentEmbeddedQuote measures the escaping path.
func BenchmarkQuoteIdentEmbeddedQuote(b *testing.B) {
	d := New()

	b.ReportAllocs()
	for b.Loop() {
		_ = d.QuoteIdent(`we"ird`)
	}
}
