package zenorm

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

func TestBackendName(t *testing.T) {
	if got := New().Name(); got != "zenorm" {
		t.Fatalf("Name() = %q, want zenorm", got)
	}
}

func TestFormatSource(t *testing.T) {
	t.Run("valid source passes through formatted", func(t *testing.T) {
		out, err := formatSource("package foo\n\nvar x = 1\n")
		if err != nil {
			t.Fatalf("formatSource: %v", err)
		}

		if !strings.Contains(string(out), "var x = 1") {
			t.Fatalf("formatSource dropped the declaration:\n%s", out)
		}
	})

	t.Run("invalid source errors", func(t *testing.T) {
		if _, err := formatSource("package foo {"); err == nil {
			t.Fatal("formatSource(invalid) = nil error, want an error")
		}
	})
}

// TestGenerateSurfacesFormatError proves an entity whose name cannot be
// represented as a Go identifier fails Generate with an error instead of
// emitting broken code. The parser/resolver never produce such names, so
// the module is built by hand.
func TestGenerateSurfacesFormatError(t *testing.T) {
	schema := &ir.Schema{Modules: []*ir.Module{{
		Entities: []*ir.Entity{{
			Name:   "a-b",
			Fields: []*ir.Field{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true}},
		}},
	}}}

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate(unrepresentable name) = nil error, want an error")
	}

	if !strings.Contains(err.Error(), "zenorm: module") {
		t.Fatalf("Generate error = %v, want the zenorm module wrapper", err)
	}
}
