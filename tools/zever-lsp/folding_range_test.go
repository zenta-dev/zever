package main

import (
	"testing"

	"go.lsp.dev/protocol"
)

// findRange reports the range in got whose StartLine matches startLine, if
// any.
func findRange(got []protocol.FoldingRange, startLine uint32) (protocol.FoldingRange, bool) {
	for _, r := range got {
		if r.StartLine == startLine {
			return r, true
		}
	}

	return protocol.FoldingRange{}, false
}

func TestFoldingRangesForEntityBody(t *testing.T) {
	// index(...) uses parens, not braces (confirmed against the grammar:
	// dsl/parser only ever opens index bodies with LPAREN), so an
	// entity with an index declaration has exactly one foldable brace pair:
	// the entity body itself. There is no nested multi-line brace block to
	// assert here.
	src := `entity Task {
	id: uuid @primary
	title: string

	index(id, title)
}
`

	got := foldingRangesFor("test.zen", []byte(src))

	want, ok := findRange(got, 0)
	if !ok {
		t.Fatalf("expected a fold range starting at line 0, got %+v", got)
	}

	if want.EndLine != 5 {
		t.Errorf("EndLine = %d, want 5", want.EndLine)
	}

	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (index(...) uses parens, not braces): %+v", len(got), got)
	}
}

func TestFoldingRangesForSingleLineBraceNotFoldable(t *testing.T) {
	src := `entity Task { id: uuid @primary }
`

	got := foldingRangesFor("test.zen", []byte(src))

	if _, ok := findRange(got, 0); ok {
		t.Errorf("single-line brace pair must not produce a fold range, got %+v", got)
	}

	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0: %+v", len(got), got)
	}
}

func TestFoldingRangesForMultiLineRolesLiteral(t *testing.T) {
	// Proves the design is "any multi-line brace pair", not an
	// entity/service/job/schedule allowlist: a bare literal set inside an
	// rpc body folds exactly like a declaration body would.
	src := `service TaskService {
	rpc DeleteTask(id: uuid) -> Task {
		http: DELETE "/tasks/{id}"
		auth: required(roles: {
			admin,
			owner,
		})
	}
}
`

	got := foldingRangesFor("test.zen", []byte(src))

	// service body: lines 0-8, rpc body: lines 1-7, roles literal: lines 3-6.
	if _, ok := findRange(got, 0); !ok {
		t.Errorf("expected service body fold range, got %+v", got)
	}

	if _, ok := findRange(got, 1); !ok {
		t.Errorf("expected rpc body fold range, got %+v", got)
	}

	rolesRange, ok := findRange(got, 3)
	if !ok {
		t.Fatalf("expected roles literal fold range starting at line 3, got %+v", got)
	}

	if rolesRange.EndLine != 6 {
		t.Errorf("roles literal EndLine = %d, want 6", rolesRange.EndLine)
	}
}

func TestFoldingRangesForMultiLineParensFold(t *testing.T) {
	src := `entity Status {
	kind: enum(
		Active,
		Done,
	)
}
`

	got := foldingRangesFor("test.zen", []byte(src))

	// The entity body (line 0) is brace-delimited and enum(...)'s
	// multi-line paren pair (lines 1-4) folds alongside it: parens pair
	// exactly like braces now.
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (entity body + multi-line parens): %+v", len(got), got)
	}

	if _, ok := findRange(got, 0); !ok {
		t.Errorf("expected entity body fold range at line 0, got %+v", got)
	}

	parenRange, ok := findRange(got, 1)
	if !ok {
		t.Fatalf("expected enum(...) paren fold range starting at line 1, got %+v", got)
	}

	if parenRange.EndLine != 4 {
		t.Errorf("paren EndLine = %d, want 4", parenRange.EndLine)
	}
}

func TestFoldingRangesForUnclosedBraceDoesNotPanic(t *testing.T) {
	src := `entity Task {
	id: uuid @primary
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on unclosed brace: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))
	if len(got) != 0 {
		t.Errorf("unclosed brace should produce no fold ranges, got %+v", got)
	}
}

func TestFoldingRangesForStrayClosingBraceDoesNotPanic(t *testing.T) {
	src := `entity Task {
	id: uuid @primary
}
}
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on stray closing brace: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))

	want, ok := findRange(got, 0)
	if !ok {
		t.Fatalf("expected the well-matched entity body pair to still fold, got %+v", got)
	}

	if want.EndLine != 2 {
		t.Errorf("EndLine = %d, want 2", want.EndLine)
	}

	if len(got) != 1 {
		t.Errorf("stray extra '}' must not itself produce a range, len(got) = %d: %+v", len(got), got)
	}
}

func TestFoldingRangesForMixedUnbalancedInput(t *testing.T) {
	// A stray leading "}" followed by a well-formed multi-line entity: the
	// well-matched pair must still be found even though an earlier close
	// had nothing to pair with.
	src := `}
entity Task {
	id: uuid @primary
}
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on mixed unbalanced input: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))

	want, ok := findRange(got, 1)
	if !ok {
		t.Fatalf("expected the well-matched entity body pair at line 1, got %+v", got)
	}

	if want.EndLine != 3 {
		t.Errorf("EndLine = %d, want 3", want.EndLine)
	}
}

func TestFoldingRangesForMultiLineValidateParensFold(t *testing.T) {
	// The motivating paren case: a multi-line @validate(...) argument list
	// folds, so long constraint lists collapse like any other block.
	src := `entity User {
	name: string @validate(
		min_len: 1,
		max_len: 64,
	)
}
`

	got := foldingRangesFor("test.zen", []byte(src))

	validateRange, ok := findRange(got, 1)
	if !ok {
		t.Fatalf("expected @validate(...) paren fold range starting at line 1, got %+v", got)
	}

	if validateRange.EndLine != 4 {
		t.Errorf("validate paren EndLine = %d, want 4", validateRange.EndLine)
	}

	if _, ok := findRange(got, 0); !ok {
		t.Errorf("expected entity body fold range at line 0, got %+v", got)
	}
}

func TestFoldingRangesForSingleLineParenNotFoldable(t *testing.T) {
	src := `entity Task {
	id: uuid @primary
	index(id)
}
`

	got := foldingRangesFor("test.zen", []byte(src))

	if _, ok := findRange(got, 2); ok {
		t.Errorf("single-line paren pair must not produce a fold range, got %+v", got)
	}

	if len(got) != 1 {
		t.Errorf("len(got) = %d, want 1 (entity body only): %+v", len(got), got)
	}
}

func TestFoldingRangesForUnclosedParenDoesNotPanic(t *testing.T) {
	src := `entity User {
	name: string @validate(
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on unclosed paren: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))

	for _, r := range got {
		if r.StartLine == 1 {
			t.Errorf("unclosed paren must not produce a fold range, got %+v", got)
		}
	}
}

func TestFoldingRangesForStrayClosingParenDoesNotPanic(t *testing.T) {
	src := `entity Task {
	id: uuid @primary
)
	index(id, title)
}
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on stray closing paren: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))

	want, ok := findRange(got, 0)
	if !ok {
		t.Fatalf("expected the well-matched entity body pair to still fold, got %+v", got)
	}

	if want.EndLine != 4 {
		t.Errorf("EndLine = %d, want 4", want.EndLine)
	}
}

func TestFoldingRangesForMismatchedDelimitersDoNotPair(t *testing.T) {
	// A ")" must not pop the brace stack and a "}" must not pop the paren
	// stack: the two stacks are independent, so each closer only pairs with
	// its own kind. Here the stray ")" is skipped and the "}" still pairs
	// with the "{" for exactly one brace range.
	src := `entity Task {
	)
}
`

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("foldingRangesFor panicked on mismatched delimiters: %v", r)
		}
	}()

	got := foldingRangesFor("test.zen", []byte(src))
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (brace pair only): %+v", len(got), got)
	}

	if got[0].StartLine != 0 || got[0].EndLine != 2 {
		t.Errorf("brace range = %+v, want 0-2", got[0])
	}
}
