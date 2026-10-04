package diag

import (
	"errors"
	"strings"
	"testing"
)

func TestPositionStringEmptyFile(t *testing.T) {
	t.Parallel()

	got := Position{}.String()
	if !strings.Contains(got, ":0:0") {
		t.Fatalf("zero Position.String() = %q, want suffix \":0:0\"", got)
	}
}

func TestDiagnosticZeroValueError(t *testing.T) {
	t.Parallel()

	var d Diagnostic
	got := d.Error()
	if !strings.Contains(got, "[]") {
		t.Fatalf("zero Diagnostic.Error() = %q, want empty phase and message", got)
	}
}

func TestListEmptyErrorIsEmptyString(t *testing.T) {
	t.Parallel()

	var list List
	if got := list.Error(); got != "" {
		t.Fatalf("empty List.Error() = %q, want \"\"", got)
	}

	if list.HasErrors() {
		t.Fatal("empty List.HasErrors() = true, want false")
	}
}

func TestListUnwrapEmpty(t *testing.T) {
	t.Parallel()

	list := List{New("p", Position{}, "m")}
	if errs := list.Unwrap(); len(errs) != 1 {
		t.Fatalf("Unwrap len = %d, want 1", len(errs))
	}

	var empty List
	if errs := empty.Unwrap(); len(errs) != 0 {
		t.Fatalf("empty Unwrap len = %d, want 0", len(errs))
	}
}

func TestListSortedEmptyAndSingle(t *testing.T) {
	t.Parallel()

	var empty List
	if got := empty.Sorted(); len(got) != 0 {
		t.Fatalf("Sorted len = %d, want 0", len(got))
	}

	single := List{New("p", Position{Line: 1}, "m")}
	if got := single.Sorted(); len(got) != 1 {
		t.Fatalf("Sorted len = %d, want 1", len(got))
	}
}

func TestListSortedStableOnEqualPositions(t *testing.T) {
	t.Parallel()

	pos := Position{File: "a.zen", Line: 1, Col: 1}
	list := List{
		New("lex", pos, "first"),
		New("parse", pos, "second"),
	}

	sorted := list.Sorted()
	if sorted[0].Msg != "first" || sorted[1].Msg != "second" {
		t.Fatalf("Sorted reordered equal-position diagnostics: %v", sorted)
	}
}

func TestListSortedOrdersByFileThenLineThenCol(t *testing.T) {
	t.Parallel()

	list := List{
		New("p", Position{File: "b.zen", Line: 1, Col: 1}, "b"),
		New("p", Position{File: "a.zen", Line: 9, Col: 2}, "a9"),
		New("p", Position{File: "a.zen", Line: 2, Col: 9}, "a2c9"),
		New("p", Position{File: "a.zen", Line: 2, Col: 3}, "a2c3"),
	}

	sorted := list.Sorted()
	want := []string{"a2c3", "a2c9", "a9", "b"}
	for i, w := range want {
		if sorted[i].Msg != w {
			t.Fatalf("sorted[%d] = %q, want %q (full: %v)", i, sorted[i].Msg, w, sorted)
		}
	}
}

func TestListHasErrorsWarningsOnly(t *testing.T) {
	t.Parallel()

	list := List{{Severity: SeverityWarning, Phase: "resolve", Msg: "drift"}}
	if list.HasErrors() {
		t.Fatal("warnings-only List.HasErrors() = true, want false")
	}
}

func TestDiagnosticUnwrapNilCause(t *testing.T) {
	t.Parallel()

	d := New("p", Position{}, "m")
	if d.Unwrap() != nil {
		t.Fatalf("Unwrap() = %v, want nil for New()", d.Unwrap())
	}
}

func TestWrapReachesCauseThroughList(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("sentinel")
	list := List{Wrap("resolve", Position{}, sentinel, "wrapped")}

	if !errors.Is(list, sentinel) {
		t.Fatal("errors.Is(List, sentinel) = false, want true via Unwrap []error")
	}
}

func TestDiagnosticErrorFormat(t *testing.T) {
	t.Parallel()

	d := New("parse", Position{File: "x.zen", Line: 3, Col: 7}, "expected %s", "identifier")
	want := "x.zen:3:7: [parse] expected identifier"
	if got := d.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
