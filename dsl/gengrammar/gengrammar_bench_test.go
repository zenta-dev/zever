package gengrammar

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// BenchmarkKeywords measures the keyword list generation.
func BenchmarkKeywords(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Keywords()
	}
}

// BenchmarkScalarTypes measures the scalar types list generation.
func BenchmarkScalarTypes(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ScalarTypes()
	}
}

// BenchmarkVimSyntax measures the Vim syntax file generation.
func BenchmarkVimSyntax(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = VimSyntax()
	}
}

// BenchmarkTmLanguage measures the TextMate grammar generation.
func BenchmarkTmLanguage(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = TmLanguage()
	}
}

// BenchmarkFiles measures the combined file generation.
func BenchmarkFiles(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Files()
	}
}

// BenchmarkLabelsAndVerbs measures the labels+verbs slice construction.
func BenchmarkLabelsAndVerbs(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		labelsAndVerbs := make([]string, 0, len(Labels)+len(Verbs))
		labelsAndVerbs = append(labelsAndVerbs, Labels...)
		labelsAndVerbs = append(labelsAndVerbs, Verbs...)
		_ = labelsAndVerbs
	}
}

// BenchmarkStringJoinKeywords measures strings.Join for keywords.
func BenchmarkStringJoinKeywords(b *testing.B) {
	keywords := Keywords()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = strings.Join(keywords, " ")
	}
}

// BenchmarkStringJoinScalars measures strings.Join for scalar types.
func BenchmarkStringJoinScalars(b *testing.B) {
	scalars := ScalarTypes()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = strings.Join(scalars, `|`)
	}
}

// BenchmarkStringJoinLabels measures strings.Join for labels.
func BenchmarkStringJoinLabels(b *testing.B) {
	labelsAndVerbs := make([]string, 0, len(Labels)+len(Verbs))
	labelsAndVerbs = append(labelsAndVerbs, Labels...)
	labelsAndVerbs = append(labelsAndVerbs, Verbs...)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = strings.Join(labelsAndVerbs, `|`)
	}
}

// BenchmarkSprintfVim measures the fmt.Fprintf for Vim syntax.
func BenchmarkSprintfVim(b *testing.B) {
	keywords := Keywords()
	scalars := ScalarTypes()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var b strings.Builder
		fmt.Fprintf(&b, "syntax keyword zenKeyword %s\n\n", strings.Join(keywords, " "))
		fmt.Fprintf(&b, "syntax keyword zenBoolean %s\n\n", strings.Join(Booleans, " "))
		fmt.Fprintf(&b, "syntax match zenType \"\\<\\%%(%s\\)\\>\"\n\n", strings.Join(scalars, `\|`))
	}
}

// BenchmarkSprintfTmLanguage measures the fmt.Sprintf for TextMate grammar.
func BenchmarkSprintfTmLanguage(b *testing.B) {
	keywords := Keywords()
	booleans := Booleans
	scalars := ScalarTypes()
	labelsAndVerbs := make([]string, 0, len(Labels)+len(Verbs))
	labelsAndVerbs = append(labelsAndVerbs, Labels...)
	labelsAndVerbs = append(labelsAndVerbs, Verbs...)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = fmt.Sprintf(tmTemplate,
			strings.Join(keywords, "|"),
			strings.Join(booleans, "|"),
			strings.Join(scalars, "|"),
			strings.Join(labelsAndVerbs, "|"),
		)
	}
}

// BenchmarkSortKeywords measures the keyword sorting.
func BenchmarkSortKeywords(b *testing.B) {
	// Create unsorted slice
	keywords := make([]string, 0, len(token.Keywords))
	for w := range token.Keywords {
		if w == "true" || w == "false" {
			continue
		}
		keywords = append(keywords, w)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sort.Strings(keywords)
	}
}
