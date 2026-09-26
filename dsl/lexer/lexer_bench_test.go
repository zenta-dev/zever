package lexer

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// BenchmarkLex measures hot Next() loop over 20k nested braces (10k "{" + 10k "}").
// This is the primary hot path: Next() → skipWhitespaceAndComments → scanPunct,
// 20k tokens, zero-alloc per token after R26 hasPrefixAt fix. Benchstat gated:
// compare old/new with -count=5, claim no alloc on hot path, no pool unless ≥5% win.
//
// The zero-alloc claim above is scoped to punctuation tokens ONLY: scanPunct's
// literals are static string constants ("{", "}", ...), never
// string(l.src[start:l.pos]). scanIdent/scanNumber/scanString (exercised by
// BenchmarkLexMixed/BenchmarkLexDuration/BenchmarkLexAstral below) all
// materialize their token literal via string([]byte) or strings.Builder,
// which always copies -- those benchmarks correctly report >0 allocs/op
// (thousands, at realistic schema-source token counts) and that is expected,
// not a regression.
func BenchmarkLex(b *testing.B) {
	src := strings.Repeat("{", 10000) + strings.Repeat("}", 10000)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New("bench.zen", []byte(src))
		for {
			tok := l.Next()
			if tok.Kind == token.EOF {
				break
			}
		}
	}
}

// BenchmarkLexAstral measures astral (outside BMP, 4-byte UTF-8, 2 UTF-16 units)
// correctness cost: 500× "😀" (U+1F600) plus a trailing entity. Col tracks
// UTF-16 units (R29), so astral runes exercise peek/advance UTF-8/UTF-16 path.
func BenchmarkLexAstral(b *testing.B) {
	src := strings.Repeat("😀", 500) + "\nentity User { id: uuid }"
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New("bench.zen", []byte(src))
		for {
			tok := l.Next()
			if tok.Kind == token.EOF {
				break
			}
		}
	}
}

// BenchmarkLexDuration measures duration-unit hot path: each numeric token
// calls matchDurationUnit → hasPrefixAt 7× (longest-first "ns","us","µs","ms"...)
// hasPrefixAt at lexer.go:385 is zero-alloc manual byte loop (R26). This bench
// proves no alloc from unit matching and captures duration suffix overhead on Next().
func BenchmarkLexDuration(b *testing.B) {
	// Mix of all 7 units plus bare ints/floats to exercise glued-number guard.
	const line = "10ns 20us 30µs 40ms 50s 60m 70h 123 45.67 1h30m "
	src := strings.Repeat(line, 800) // ~ 800* (~45B) ~36KB, ~8000 tokens
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New("bench.zen", []byte(src))
		for {
			tok := l.Next()
			if tok.Kind == token.EOF {
				break
			}
		}
	}
}

// BenchmarkLexDurationUnits isolates per-unit cost for each duration suffix
// individually. Each sub-bench lexes 5k repetitions of "<n><unit>" to surface
// any regression in hasPrefixAt ordering (longest match first).
func BenchmarkLexDurationUnits(b *testing.B) {
	cases := []struct {
		name string
		unit string
	}{
		{"ns", "ns"}, {"us", "us"}, {"mus", "µs"}, {"ms", "ms"}, {"s", "s"}, {"m", "m"}, {"h", "h"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			src := strings.Repeat("42"+tc.unit+" ", 5000)
			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			for i := 0; i < b.N; i++ {
				l := New("bench.zen", []byte(src))
				for {
					tok := l.Next()
					if tok.Kind == token.EOF {
						break
					}
				}
			}
		})
	}
}

// BenchmarkLexMixed models a realistic schema snippet repeated to ~30KB,
// covering ident/keyword, punctuation, string, int/float/duration, and comments.
// Useful as a holistic Next() budget neben the synthetic micro-benches above.
func BenchmarkLexMixed(b *testing.B) {
	const snippet = `
// User entity
entity User {
  id: uuid @pk @auto
  name: string @unique
  age: int
  score: float
  ttl: duration // e.g. 5m
  bio: string
}
`
	src := strings.Repeat(snippet, 300)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		l := New("bench.zen", []byte(src))
		for {
			tok := l.Next()
			if tok.Kind == token.EOF {
				break
			}
		}
	}
}
