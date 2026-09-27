package parser

import (
	"strings"
	"testing"
)

// TestParseValueDepthGuardDoesNotStackOverflow proves the fix for the
// parseValue -> parseCallOrIdent -> parseArgList -> parseArg -> parseValue
// mutual-recursion cycle: a deeply nested, otherwise-well-formed call-value
// expression must not stack-overflow the process. Instead, once the nesting
// exceeds the parser's guard threshold, parsing must abort that value with a
// recorded diag.Diagnostic and unwind cleanly, per the package's
// never-panic, always-returns-a-file contract.
//
// The nesting depth here (well past any realistic threshold) is built via
// string concatenation of a valid call chain (`nested(nested(nested(...)))`),
// distinct from the existing fuzz seeds' bare bracket-character nesting
// (e.g. "((((("), which never reaches this recursive path at all because a
// bare "(" isn't a valid Value start.
func TestParseValueDepthGuardDoesNotStackOverflow(t *testing.T) {
	const depth = 5000

	src := "entity Foo { id: uuid @validate(" +
		strings.Repeat("nested(", depth) + "1" + strings.Repeat(")", depth) +
		") }"

	file, msgs := parseSrc(t, src)

	if file == nil {
		t.Fatalf("ParseFile returned a nil *ast.File for deeply nested input, want always-non-nil per package doc contract")
	}

	found := false

	for _, m := range msgs {
		if strings.Contains(m, "max nesting depth exceeded") {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected a diagnostic mentioning 'max nesting depth exceeded', got: %v", msgs)
	}
}
