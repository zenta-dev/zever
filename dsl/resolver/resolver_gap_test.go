package resolver

import (
	"sort"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
)

// TestScalarTypeNamesCanonical pins the public contract of the single source
// of truth for scalar type names: a sorted, duplicate-free, enum-excluded
// list that exactly matches the scalarTypes table.
func TestScalarTypeNamesCanonical(t *testing.T) {
	t.Parallel()

	got := ScalarTypeNames()

	want := []string{
		"bool", "bytes", "date", "datetime", "float32", "float64",
		"int32", "int64", "json", "string", "timestamp", "uuid",
	}

	if len(got) != len(want) {
		t.Fatalf("ScalarTypeNames() len = %d (%v), want %d", len(got), got, len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ScalarTypeNames()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	if !sort.StringsAreSorted(got) {
		t.Fatalf("ScalarTypeNames() = %v, not sorted", got)
	}

	for _, name := range got {
		if name == "enum" {
			t.Fatal("ScalarTypeNames() must exclude enum: it resolves via a separate arg-list path")
		}
	}
}

// TestScalarTypeNamesResolveAsScalars proves every advertised name actually
// resolves through resolveFieldType to its scalar type, so codegen grammars
// derived from ScalarTypeNames can never drift from the resolver.
func TestScalarTypeNamesResolveAsScalars(t *testing.T) {
	t.Parallel()

	for _, name := range ScalarTypeNames() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ft, diag := resolveFieldType(&ast.TypeExpr{Name: name}, nil)
			if diag != nil {
				t.Fatalf("resolveFieldType(%q) diagnostic: %v", name, diag)
			}

			if ft.EnumName != "" {
				t.Fatalf("resolveFieldType(%q) EnumName = %q, want empty", name, ft.EnumName)
			}

			if _, ok := scalarTypes[name]; !ok {
				t.Fatalf("resolveFieldType(%q) resolved but name absent from scalarTypes", name)
			}
		})
	}
}

// BenchmarkScalarTypeNames measures building the canonical name slice, which
// grammar generators call at startup.
func BenchmarkScalarTypeNames(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = ScalarTypeNames()
	}
}
