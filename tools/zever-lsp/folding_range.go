package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/token"
)

// foldingRangesFor returns one FoldingRange per matched "{...}" brace pair
// or "(...)" paren pair that spans more than one source line, computed by a
// single pass over the token stream (lexAll, shared with format.go).
//
// Braces and parens are paired with independent stacks of opening-token
// lines rather than format.go's scalar depth counter: format.go only ever
// needs "what depth is this line at", never "which specific open matches
// this specific close". Folding needs the latter, since a close on line N
// must be matched back to its own open's line, not merely to "some earlier
// line one level up". Independent stacks keep the two delimiter kinds from
// interfering: a "(" closed by "}" (or vice versa) leaves both stacks
// untouched for that token, exactly like a stray closer.
//
// This deliberately does not special-case declaration kinds. Every brace
// pair in the grammar -- entity/service/job/schedule bodies, RPC
// bodies, and literal sets like "roles: {admin, editor}" -- is foldable the
// same way, because the DSL has no brace usage that isn't a block or a
// brace-delimited literal worth collapsing. Parens cover the call-like
// constructs -- index(...), enum(...) type args, @validate(...),
// @default(...), dispatch: Job(...) -- so a multi-line argument list folds
// the same way a multi-line body does. Single-line pairs of either kind
// never fold.
//
// Malformed input degrades gracefully instead of panicking: an unclosed "{"
// or "(" left on the stack at EOF never produces a range (there is no
// matching close to measure), and a stray "}" or ")" with an empty stack is
// simply skipped.
func foldingRangesFor(path string, src []byte) []protocol.FoldingRange {
	tokens, _ := lexAll(path, src)

	var (
		ranges     []protocol.FoldingRange
		braceStack []int // 1-based source lines of unmatched "{" tokens
		parenStack []int // 1-based source lines of unmatched "(" tokens
	)

	for _, tok := range tokens {
		switch tok.Kind { //nolint:exhaustive // only braces and parens affect nesting here
		case token.LBRACE:
			braceStack = append(braceStack, tok.Pos.Line)
		case token.RBRACE:
			openLine, ok := popFoldLine(&braceStack)
			if !ok {
				continue // stray close: no matching open, nothing to fold
			}

			appendFoldRange(&ranges, openLine, tok.Pos.Line)
		case token.LPAREN:
			parenStack = append(parenStack, tok.Pos.Line)
		case token.RPAREN:
			openLine, ok := popFoldLine(&parenStack)
			if !ok {
				continue // stray close: no matching open, nothing to fold
			}

			appendFoldRange(&ranges, openLine, tok.Pos.Line)
		}
	}

	return ranges
}

// popFoldLine pops the innermost unmatched open line off stack, reporting
// whether one existed. The caller passes either the brace or the paren
// stack; mismatched closers never touch the other stack.
func popFoldLine(stack *[]int) (int, bool) {
	if len(*stack) == 0 {
		return 0, false
	}

	openLine := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]

	return openLine, true
}

// appendFoldRange records a fold spanning openLine to closeLine (both
// 1-based), unless the pair sits on a single line, which is never foldable.
func appendFoldRange(ranges *[]protocol.FoldingRange, openLine, closeLine int) {
	if openLine == closeLine {
		return // single-line pair: not foldable
	}

	*ranges = append(*ranges, protocol.FoldingRange{
		// source line numbers are small and non-negative
		StartLine: uint32(openLine - 1),
		// source line numbers are small and non-negative
		EndLine: uint32(closeLine - 1),
	})
}
