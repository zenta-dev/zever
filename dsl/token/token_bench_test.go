package token

import "testing"

// BenchmarkKindString measures the Kind.String() switch across every defined
// kind in one pass; the switch is small enough to stay branch-predictable.
func BenchmarkKindString(b *testing.B) {
	kinds := []Kind{
		EOF, ILLEGAL, IDENT, INT, FLOAT, DURATION, STRING,
		LBRACE, RBRACE, LPAREN, RPAREN, LBRACK, RBRACK,
		COLON, COMMA, ARROW, AT, MINUS, QUESTION,
		ENTITY, SERVICE, JOB, SCHEDULE, RPC, INDEX, ENUM,
		HAS_MANY, HAS_ONE, BELONGS_TO, MANY_TO_MANY, MESSAGE, TRUE, FALSE,
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, k := range kinds {
			_ = k.String()
		}
	}
}

// BenchmarkKeywordsLookup measures the map hit path for a reserved word and
// the miss path for a plain type name, the two lookups the lexer performs
// per identifier.
func BenchmarkKeywordsLookup(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Keywords["entity"]
		_ = Keywords["uuid"]
	}
}

// BenchmarkKeywordsMiss measures lookup of an absent key (type names,
// contextual labels, HTTP verbs are deliberately not reserved).
func BenchmarkKeywordsMiss(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = Keywords["GET"]
	}
}
