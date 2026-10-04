package diag

import "testing"

// BenchmarkPositionString measures the position formatting hot path every
// diagnostic pays before it can be rendered.
func BenchmarkPositionString(b *testing.B) {
	p := Position{File: "schema/app.zen", Line: 42, Col: 7}

	b.ReportAllocs()
	for b.Loop() {
		_ = p.String()
	}
}

// BenchmarkDiagnosticError measures single-diagnostic rendering, including
// its phase prefix.
func BenchmarkDiagnosticError(b *testing.B) {
	d := New("resolve", Position{File: "a.zen", Line: 1, Col: 2}, "unresolved reference")

	b.ReportAllocs()
	for b.Loop() {
		_ = d.Error()
	}
}

// BenchmarkNew measures diagnostic construction.
func BenchmarkNew(b *testing.B) {
	p := Position{File: "a.zen", Line: 1, Col: 2}

	b.ReportAllocs()
	for b.Loop() {
		_ = New("parse", p, "expected identifier")
	}
}

// BenchmarkListSorted measures the sort every consumer (CLI, tests) applies
// before rendering diagnostics in source order.
func BenchmarkListSorted(b *testing.B) {
	list := List{
		New("resolve", Position{File: "a.zen", Line: 9, Col: 1}, "third"),
		New("lex", Position{File: "a.zen", Line: 2, Col: 5}, "first"),
		New("parse", Position{File: "a.zen", Line: 5, Col: 3}, "second"),
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = list.Sorted()
	}
}

// BenchmarkListHasErrors walks mixed-severity lists.
func BenchmarkListHasErrors(b *testing.B) {
	list := List{
		New("resolve", Position{File: "a.zen", Line: 1, Col: 1}, "error"),
		{Pos: Position{File: "a.zen", Line: 2, Col: 1}, Severity: SeverityWarning, Phase: "resolve", Msg: "warning"},
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = list.HasErrors()
	}
}

// BenchmarkListError measures joined multi-diagnostic rendering.
func BenchmarkListError(b *testing.B) {
	list := List{
		New("lex", Position{File: "a.zen", Line: 1, Col: 1}, "illegal character"),
		New("parse", Position{File: "a.zen", Line: 2, Col: 1}, "expected identifier"),
		New("resolve", Position{File: "a.zen", Line: 3, Col: 1}, "unresolved reference"),
	}

	b.ReportAllocs()
	for b.Loop() {
		_ = list.Error()
	}
}
