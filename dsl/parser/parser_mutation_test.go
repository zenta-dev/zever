package parser

import (
	"strings"
	"testing"
)

// TestParseValueDepthBoundary pins the exact maxValueDepth boundary the
// depth guard enforces: 199 nested call values parse cleanly, while 200
// nested calls record the "max nesting depth exceeded" diagnostic. The
// pre-existing depth test only uses depth 5000, so an off-by-one in the
// guard (`>` vs `>=`) survived it; this test kills that mutant.
func TestParseValueDepthBoundary(t *testing.T) {
	t.Parallel()

	build := func(n int) string {
		return "entity Foo { id: uuid @validate(" +
			strings.Repeat("nested(", n) + "1" + strings.Repeat(")", n) +
			") }"
	}

	if _, msgs := parseSrc(t, build(199)); len(msgs) != 0 {
		t.Fatalf("depth 199 diags = %v, want clean parse", msgs)
	}

	_, msgs := parseSrc(t, build(200))

	found := false

	for _, m := range msgs {
		if strings.Contains(m, "max nesting depth exceeded") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("depth 200 missing depth diagnostic, got: %v", msgs)
	}
}

// TestParseAttributeEmptyParens proves an attribute with an explicitly empty
// argument list (`@a()`) parses to an attribute with zero args and no
// diagnostics, rather than failing the argument parse. Removing the
// empty-parens fast path from parseAttribute survived the suite; this test
// kills that mutant.
func TestParseAttributeEmptyParens(t *testing.T) {
	t.Parallel()

	file, msgs := parseSrc(t, `entity E { f: string @a() }`)
	if len(msgs) != 0 {
		t.Fatalf("diags = %v, want clean parse", msgs)
	}

	attr := firstFieldAttr(t, file)
	if attr.Name != "a" {
		t.Fatalf("attr.Name = %q, want a", attr.Name)
	}

	if len(attr.Args) != 0 {
		t.Fatalf("len(args) = %d, want 0", len(attr.Args))
	}
}
