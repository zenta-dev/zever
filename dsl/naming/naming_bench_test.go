package naming

import "testing"

// BenchmarkPascalCase measures the snake_case and camelCase branches over
// representative identifier shapes.
func BenchmarkPascalCase(b *testing.B) {
	inputs := []string{"user", "user_profile", "userProfile", "User", "created_at", "HTTPResponseCode"}

	b.ReportAllocs()
	for b.Loop() {
		for _, s := range inputs {
			_ = PascalCase(s)
		}
	}
}

// BenchmarkScreamingSnake measures the acronym-aware snake conversion plus
// uppercasing.
func BenchmarkScreamingSnake(b *testing.B) {
	inputs := []string{"user", "userProfile", "User", "created_at", "APIKey", "HTTPServer"}

	b.ReportAllocs()
	for b.Loop() {
		for _, s := range inputs {
			_ = ScreamingSnake(s)
		}
	}
}

// BenchmarkSnakeCase measures the lowercase snake conversion the zenorm
// backend uses for output paths.
func BenchmarkSnakeCase(b *testing.B) {
	inputs := []string{"user", "userProfile", "User", "created_at", "APIKey", "HTTPServer"}

	b.ReportAllocs()
	for b.Loop() {
		for _, s := range inputs {
			_ = SnakeCase(s)
		}
	}
}

// BenchmarkPluralizeNaive measures each pluralization branch: y→ies,
// s/x/ch/sh→es, default +s.
func BenchmarkPluralizeNaive(b *testing.B) {
	inputs := []string{"user", "category", "box", "church", "dish", "status"}

	b.ReportAllocs()
	for b.Loop() {
		for _, s := range inputs {
			_ = PluralizeNaive(s)
		}
	}
}
