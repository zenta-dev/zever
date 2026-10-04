package gengrammar

import "testing"

// BenchmarkVimSyntax renders the whole vim grammar document.
func BenchmarkVimSyntax(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = VimSyntax()
	}
}

// BenchmarkTmLanguage renders the whole TextMate grammar JSON document.
func BenchmarkTmLanguage(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = TmLanguage()
	}
}

// BenchmarkFiles renders both grammar documents in one call.
func BenchmarkFiles(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Files()
	}
}

// BenchmarkKeywords measures the token.Keywords drain plus sort.
func BenchmarkKeywords(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Keywords()
	}
}

// BenchmarkScalarTypes measures the resolver.ScalarTypeNames() delegation.
func BenchmarkScalarTypes(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = ScalarTypes()
	}
}
