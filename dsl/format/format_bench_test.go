package format

import (
	"os"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// BenchmarkFormatAppFixture formats the canonical compile/testdata/app.zen
// fixture end to end: lex, line states, indent/blank/spacing edits, apply.
func BenchmarkFormatAppFixture(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if _, err := Format(src); err != nil {
			b.Fatalf("Format: %v", err)
		}
	}
}

// BenchmarkEditsAppFixture measures the edit-computation half of Format
// (lex + line states + the three edit passes), excluding Apply.
func BenchmarkEditsAppFixture(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if _, ok := Edits("app.zen", src); !ok {
			b.Fatal("Edits reported dirty for a clean fixture")
		}
	}
}

// BenchmarkLexAll measures the lexer drain plus error check.
func BenchmarkLexAll(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if _, clean := LexAll("app.zen", src); !clean {
			b.Fatal("LexAll reported errors for a clean fixture")
		}
	}
}

// BenchmarkCanonicalGap measures the pairwise gap classifier over a
// representative mix of adjacent token kinds.
func BenchmarkCanonicalGap(b *testing.B) {
	pairs := [][2]token.Token{
		{token.Token{Kind: token.IDENT, Lit: "id"}, token.Token{Kind: token.COLON}},
		{token.Token{Kind: token.COLON}, token.Token{Kind: token.IDENT, Lit: "uuid"}},
		{token.Token{Kind: token.AT}, token.Token{Kind: token.IDENT, Lit: "primary"}},
		{token.Token{Kind: token.IDENT, Lit: "name"}, token.Token{Kind: token.COMMA}},
		{token.Token{Kind: token.IDENT, Lit: "x"}, token.Token{Kind: token.LPAREN}},
		{token.Token{Kind: token.ARROW}, token.Token{Kind: token.LBRACE}},
		{token.Token{Kind: token.QUESTION}, token.Token{Kind: token.RBRACE}},
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, p := range pairs {
			_, _ = CanonicalGap(p[0], p[1])
		}
	}
}

// BenchmarkSplitLines measures line splitting over a large source.
func BenchmarkSplitLines(b *testing.B) {
	src := []byte(strings.Repeat("entity User {\n  id: uuid @primary\n}\n", 2000))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = SplitLines(src)
	}
}

// BenchmarkApply measures edit application over a source with many edits.
func BenchmarkApply(b *testing.B) {
	src := []byte(strings.Repeat("entity    User   {\nid:   uuid\n}\n", 500))

	edits, ok := Edits("messy.zen", src)
	if !ok {
		b.Fatal("Edits reported dirty")
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		_ = Apply(src, edits)
	}
}
