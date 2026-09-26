package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/token"
)

// foldingRangesFor returns one FoldingRange per matched "{...}" brace pair
// that spans more than one source line, computed by a single pass over the
// token stream (lexAll, shared with format.go).
//
// Braces are paired with a stack of opening-token lines rather than
// format.go's scalar depth counter: format.go only ever needs "what depth is
// this line at", never "which specific open matches this specific close".
// Folding needs the latter, since a close on line N must be matched back to
// its own open's line, not merely to "some earlier line one level up".
//
// This deliberately does not special-case declaration kinds. Every brace
// pair in the grammar -- entity/service/job/schedule bodies, RPC
// bodies, and literal sets like "roles: {admin, editor}" -- is foldable the
// same way, because the DSL has no brace usage that isn't a block or a
// brace-delimited literal worth collapsing. Parens ("(...)", used for e.g.
// index(...) and enum(...) type args) are ignored entirely: only LBRACE/
// RBRACE push and pop the stack.
//
// Malformed input degrades gracefully instead of panicking: an unclosed "{"
// left on the stack at EOF never produces a range (there is no matching
// close to measure), and a stray "}" with an empty stack is simply skipped.
func foldingRangesFor(path string, src []byte) []protocol.FoldingRange {
	tokens, _ := lexAll(path, src)

	var (
		ranges []protocol.FoldingRange
		stack  []int // 1-based source lines of unmatched "{" tokens
	)

	for _, tok := range tokens {
		switch tok.Kind { //nolint:exhaustive // only braces affect nesting here
		case token.LBRACE:
			stack = append(stack, tok.Pos.Line)
		case token.RBRACE:
			if len(stack) == 0 {
				continue // stray close: no matching open, nothing to fold
			}

			openLine := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			closeLine := tok.Pos.Line
			if openLine == closeLine {
				continue // single-line pair: not foldable
			}

			ranges = append(ranges, protocol.FoldingRange{
				//nolint:gosec // source line numbers are small and non-negative
				StartLine: uint32(openLine - 1),
				//nolint:gosec // source line numbers are small and non-negative
				EndLine: uint32(closeLine - 1),
			})
		}
	}

	return ranges
}
