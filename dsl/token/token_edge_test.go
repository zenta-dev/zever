package token

import "testing"

func TestKindZeroValueIsEOF(t *testing.T) {
	t.Parallel()

	var k Kind
	if k != EOF {
		t.Fatalf("zero Kind = %d, want EOF (%d)", k, EOF)
	}
}

func TestKindStringUnknownKindDefaultsToIllegal(t *testing.T) {
	t.Parallel()

	if got := Kind(-1).String(); got != "ILLEGAL" {
		t.Fatalf("Kind(-1).String() = %q, want \"ILLEGAL\"", got)
	}

	if got := Kind(1 << 20).String(); got != "ILLEGAL" {
		t.Fatalf("Kind(1<<20).String() = %q, want \"ILLEGAL\"", got)
	}
}

func TestKeywordsEmptyStringAbsent(t *testing.T) {
	t.Parallel()

	if _, ok := Keywords[""]; ok {
		t.Fatal("Keywords[\"\"] present, want absent")
	}
}

func TestKeywordsAllValuesAreKeywordKinds(t *testing.T) {
	t.Parallel()

	for word, kind := range Keywords {
		if kind.String() == "ILLEGAL" {
			t.Fatalf("Keywords[%q] = %d, which String()s as ILLEGAL", word, kind)
		}
	}
}

func TestTokenZeroValue(t *testing.T) {
	t.Parallel()

	var tok Token
	if tok.Kind != EOF || tok.Lit != "" {
		t.Fatalf("zero Token = %+v, want {Kind: EOF, Lit: \"\"}", tok)
	}
}
