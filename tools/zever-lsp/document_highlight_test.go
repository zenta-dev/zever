package main

import (
	"sort"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

// parseHighlightFixture parses src as a standalone file, exactly the way
// highlightsAt's caller (Server.documentHighlight) receives its *ast.File --
// via the parser directly, no server/workspace machinery needed since
// highlightsAt only ever looks at the one file it's given.
func parseHighlightFixture(t *testing.T, src string) *ast.File {
	t.Helper()

	file, _ := parser.New("fixture.zen", []byte(src)).ParseFile()
	if file == nil {
		t.Fatalf("failed to parse fixture:\n%s", src)
	}

	return file
}

func sortHighlights(hs []protocol.DocumentHighlight) {
	sort.Slice(hs, func(i, j int) bool {
		a, b := hs[i].Range.Start, hs[j].Range.Start
		if a.Line != b.Line {
			return a.Line < b.Line
		}

		return a.Character < b.Character
	})
}

func TestHighlightsAtEntityDeclaredAndReferencedInSameFile(t *testing.T) {
	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"}\n\n" +
		"entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: User @foreign_key(user_id)\n" +
		"}\n"

	file := parseHighlightFixture(t, src)

	// Cursor on the entity's own declaration name (line 0, "User" at col 7).
	got := highlightsAt(file, protocol.Position{Line: 0, Character: 7})

	sortHighlights(got)

	if len(got) != 2 {
		t.Fatalf("highlightsAt() = %d highlights, want 2 (declaration + relation target): %+v", len(got), got)
	}

	// Declaration on line 0, relation target on line 7 (0-based: blank line
	// 3 separates the two entity blocks).
	if got[0].Range.Start.Line != 0 {
		t.Errorf("first highlight on line %d, want 0 (declaration)", got[0].Range.Start.Line)
	}

	if got[1].Range.Start.Line != 7 {
		t.Errorf("second highlight on line %d, want 7 (relation target)", got[1].Range.Start.Line)
	}

	for _, h := range got {
		if h.Kind != protocol.DocumentHighlightKindText {
			t.Errorf("highlight kind = %v, want DocumentHighlightKindText", h.Kind)
		}
	}

	// Cursor on the reference itself must resolve to the same set.
	gotFromRef := highlightsAt(file, protocol.Position{Line: 7, Character: 20})
	sortHighlights(gotFromRef)

	if len(gotFromRef) != 2 {
		t.Fatalf("highlightsAt() from reference = %d highlights, want 2", len(gotFromRef))
	}
}

func TestHighlightsAtLocalOccurrencesWithoutDeclaration(t *testing.T) {
	// User is declared in some other file the LSP never shows us here --
	// this file only references it via a relation target. highlightsAt must
	// still resolve the cursor to the symbol name and return the local
	// occurrence(s), proving it doesn't require seeing the declaration.
	src := "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: User @foreign_key(user_id)\n" +
		"}\n"

	file := parseHighlightFixture(t, src)

	got := highlightsAt(file, protocol.Position{Line: 3, Character: 20})

	if len(got) != 1 {
		t.Fatalf("highlightsAt() = %d highlights, want 1 (the local relation-target reference): %+v", len(got), got)
	}

	if got[0].Range.Start.Line != 3 {
		t.Errorf("highlight on line %d, want 3", got[0].Range.Start.Line)
	}
}

func TestHighlightsAtFieldDoesNotBleedAcrossEntities(t *testing.T) {
	src := "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"\tindex(email)\n" +
		"}\n\n" +
		"entity Order {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"}\n"

	file := parseHighlightFixture(t, src)

	// Cursor on User.email's declaration (line 2).
	got := highlightsAt(file, protocol.Position{Line: 2, Character: 2})

	sortHighlights(got)

	if len(got) != 2 {
		t.Fatalf("highlightsAt() = %d highlights, want 2 (User.email declaration + index column): %+v", len(got), got)
	}

	for _, h := range got {
		if h.Range.Start.Line == 7 {
			t.Errorf("highlight bled into Order.email at line 7: %+v", got)
		}
	}

	// And the reverse: cursor on Order.email must not pick up User's refs.
	gotOrder := highlightsAt(file, protocol.Position{Line: 7, Character: 2})

	if len(gotOrder) != 1 {
		t.Fatalf("highlightsAt() on Order.email = %d highlights, want 1 (its own declaration only): %+v", len(gotOrder), gotOrder)
	}

	if gotOrder[0].Range.Start.Line != 7 {
		t.Errorf("Order.email highlight on line %d, want 7", gotOrder[0].Range.Start.Line)
	}
}

func TestHighlightsAtNonRenameablePositionReturnsEmpty(t *testing.T) {
	src := "job CleanupJob(id: uuid) {\n" +
		"\tqueue: \"default\"\n" +
		"}\n"

	file := parseHighlightFixture(t, src)

	// Cursor inside the queue string literal: not a renameable symbol.
	got := highlightsAt(file, protocol.Position{Line: 1, Character: 10})

	if len(got) != 0 {
		t.Fatalf("highlightsAt() on non-renameable position = %+v, want empty", got)
	}
}

// highlightServiceSrc holds a service with an RPC plus a job with a
// dispatching schedule in ONE file, since highlightsAt is single-file
// scoped: every arm below resolves and collects within this same text.
const highlightServiceSrc = "service UserService {\n" +
	"\trpc GetUser(id: uuid) -> User {\n" +
	"\t\tauth: none\n" +
	"\t}\n" +
	"}\n\n" +
	"job CleanupJob(id: uuid) {\n" +
	"\tqueue: \"default\"\n" +
	"}\n\n" +
	"schedule Nightly {\n" +
	"\tcron: \"0 0 * * *\"\n" +
	"\tdispatch: CleanupJob()\n" +
	"}\n"

func TestHighlightsAtJobHighlightsDeclarationAndDispatch(t *testing.T) {
	file := parseHighlightFixture(t, highlightServiceSrc)

	// Cursor on the job's own declaration name (line 6).
	got := highlightsAt(file, protocol.Position{Line: 6, Character: 5})
	sortHighlights(got)

	if len(got) != 2 {
		t.Fatalf("highlightsAt() = %d highlights, want 2 (declaration + dispatch): %+v", len(got), got)
	}

	if got[0].Range.Start.Line != 6 {
		t.Errorf("first highlight on line %d, want 6 (declaration)", got[0].Range.Start.Line)
	}

	if got[1].Range.Start.Line != 12 {
		t.Errorf("second highlight on line %d, want 12 (dispatch)", got[1].Range.Start.Line)
	}

	for _, h := range got {
		if h.Kind != protocol.DocumentHighlightKindText {
			t.Errorf("highlight kind = %v, want DocumentHighlightKindText", h.Kind)
		}
	}
}

func TestHighlightsAtServiceName(t *testing.T) {
	file := parseHighlightFixture(t, highlightServiceSrc)

	got := highlightsAt(file, cursorOn(t, highlightServiceSrc, 0, "UserService"))

	if len(got) != 1 {
		t.Fatalf("highlightsAt() = %d highlights, want 1 (the declaration): %+v", len(got), got)
	}

	if got[0].Range.Start.Line != 0 {
		t.Errorf("highlight on line %d, want 0", got[0].Range.Start.Line)
	}
}

func TestHighlightsAtRpcName(t *testing.T) {
	file := parseHighlightFixture(t, highlightServiceSrc)

	got := highlightsAt(file, cursorOn(t, highlightServiceSrc, 1, "GetUser"))

	if len(got) != 1 {
		t.Fatalf("highlightsAt() = %d highlights, want 1 (the declaration): %+v", len(got), got)
	}

	if got[0].Range.Start.Line != 1 {
		t.Errorf("highlight on line %d, want 1", got[0].Range.Start.Line)
	}
}
