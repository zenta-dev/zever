package main

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// selectionEntityDoc is a minimal two-declaration document whose lines are
// counted out below:
//
//	0: entity User {
//	1: \tid: uuid @primary
//	2: }
//
// The `uuid` on line 1 starts at column 6 (tab, "id", ':', space).
const selectionEntityDoc = "entity User {\n\tid: uuid @primary\n}\n"

// selectionDocURI is the URI the handler-level tests register the fixture
// under.
const selectionDocURI = "file:///tmp/zever-lsp-selection.zen"

// wantSelectionRange builds one absolute LSP range, the shape every chain
// assertion below compares against.
func wantSelectionRange(line, startChar, endLine, endChar uint32) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: line, Character: startChar},
		End:   protocol.Position{Line: endLine, Character: endChar},
	}
}

// assertSelectionChain walks parent from innermost to outermost and checks
// each range against want, failing the test on the first mismatch and
// reporting how deep the chain actually went.
func assertSelectionChain(t *testing.T, got []protocol.SelectionRange, want []protocol.Range) {
	t.Helper()

	if len(got) == 0 {
		t.Fatalf("selection range chain empty")
		return
	}

	// Climb the explicit parent links, as a client does, instead of trusting
	// the slice order: the chain's shape IS the contract.
	node := got[0]
	for i, rng := range want {
		if node.Range != rng {
			t.Errorf("chain[%d].Range = %+v, want %+v", i, node.Range, rng)
		}

		if i == len(want)-1 {
			break
		}

		if node.Parent == nil {
			t.Fatalf("chain ended after %d links, want %d", i+1, len(want))

			return
		}

		node = *node.Parent
	}
}

func TestSelectionRangeChainInsideEntity(t *testing.T) {
	tokens, _ := lexAll("selection.zen", []byte(selectionEntityDoc))

	// Cursor inside the field's type token: the chain must be
	// token -> declaration head line -> entity body -> whole document.
	got := selectionRangeAt(tokens, selectionEntityDoc, protocol.Position{Line: 1, Character: 6})

	assertSelectionChain(t, []protocol.SelectionRange{got}, []protocol.Range{
		// The token itself: "uuid", tab included the column is 6 wide 4.
		wantSelectionRange(1, 5, 1, 9),
		// The declaration head at line granularity: the whole field line.
		wantSelectionRange(1, 0, 1, 18),
		// The entity body: the open brace's line through the close brace's.
		wantSelectionRange(0, 0, 2, 1),
		// The whole document: every line through the last line's text.
		wantSelectionRange(0, 0, 3, 0),
	})
}

func TestSelectionRangeChainFromKeyword(t *testing.T) {
	// A cursor on the declaration keyword itself must still produce a
	// contained chain: the keyword precedes the entity's braces, so the
	// block layer it can descend no further than the document.
	tokens, _ := lexAll("selection.zen", []byte(selectionEntityDoc))

	got := selectionRangeAt(tokens, selectionEntityDoc, protocol.Position{Line: 0, Character: 3})

	assertSelectionChain(t, []protocol.SelectionRange{got}, []protocol.Range{
		wantSelectionRange(0, 0, 0, 6),  // the "entity" keyword token
		wantSelectionRange(0, 0, 0, 13), // the declaration head line
		wantSelectionRange(0, 0, 3, 0),  // whole document: no enclosing block
	})
}

func TestSelectionRangeMultiplePositions(t *testing.T) {
	s := NewServer()

	openDoc(t, s, selectionDocURI, selectionEntityDoc)

	// Two positions in one request, each resolving to a different-shaped
	// chain: the field type token nests fully (four layers), the close
	// brace collapses its head line, block and token layer overlaps (the
	// token, head line and block range all span the same single close line,
	// so only the block and document layers survive).
	res, err := s.SelectionRange(t.Context(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(selectionDocURI)},
		Positions: []protocol.Position{
			{Line: 1, Character: 6},
			{Line: 2, Character: 0},
		},
	})
	if err != nil {
		t.Fatalf("SelectionRange: %v", err)
	}

	if len(res) != 2 {
		t.Fatalf("SelectionRange resolved %d results, want one per position: %+v", len(res), res)
	}

	assertSelectionChain(t, res[:1], []protocol.Range{
		wantSelectionRange(1, 5, 1, 9),
		wantSelectionRange(1, 0, 1, 18),
		wantSelectionRange(0, 0, 2, 1),
		wantSelectionRange(0, 0, 3, 0),
	})

	assertSelectionChain(t, res[1:], []protocol.Range{
		wantSelectionRange(2, 0, 2, 1),
		wantSelectionRange(0, 0, 2, 1),
		wantSelectionRange(0, 0, 3, 0),
	})
}

func TestSelectionRangeUnknownDocument(t *testing.T) {
	s := NewServer()

	res, err := s.SelectionRange(t.Context(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(selectionDocURI)},
		Positions:    []protocol.Position{{Line: 0, Character: 0}},
	})
	if err != nil {
		t.Fatalf("SelectionRange on unknown doc: %v", err)
	}

	if len(res) != 0 {
		t.Errorf("SelectionRange on unknown doc = %+v, want empty", res)
	}
}

func TestSelectionRangePositionOffEnd(t *testing.T) {
	s := NewServer()

	openDoc(t, s, selectionDocURI, selectionEntityDoc)

	// A line past the document's end contains nothing; the result must be
	// the spec's degenerate case: one SelectionRange whose range starts and
	// ends exactly at the requested position, with no parent.
	res, err := s.SelectionRange(t.Context(), &protocol.SelectionRangeParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(selectionDocURI)},
		Positions:    []protocol.Position{{Line: 9, Character: 0}},
	})
	if err != nil {
		t.Fatalf("SelectionRange off end: %v", err)
	}

	if len(res) != 1 {
		t.Fatalf("SelectionRange off end resolved %d results, want 1: %+v", len(res), res)
	}

	if res[0].Range.Start.Line != 9 || res[0].Range.End.Line != 9 {
		t.Errorf("off-end range = %+v, want a degenerate range anchored on line 9", res[0].Range)
	}

	if res[0].Parent != nil {
		t.Errorf("off-end range has a parent %+v, want none", res[0].Parent)
	}
}

func TestSelectionRangePositionsContainTheirRanges(t *testing.T) {
	// The spec's base rule, checked over a spread of positions: every
	// result's range must contain its position, and walking .Parent outward
	// must be non-decreasing in extent (each parent contains its child).
	tokens, _ := lexAll("selection.zen", []byte(selectionEntityDoc))

	positions := []protocol.Position{
		{Line: 0, Character: 0},
		{Line: 0, Character: 3},
		{Line: 0, Character: 13},
		{Line: 1, Character: 1},  // the field name
		{Line: 1, Character: 6},  // the scalar type
		{Line: 1, Character: 15}, // the attribute name
		{Line: 2, Character: 0},  // the close brace
		{Line: 3, Character: 0},  // the trailing empty line
	}

	for _, pos := range positions {
		got := selectionRangeAt(tokens, selectionEntityDoc, pos)

		if !rangeContainsPos(got.Range, pos) {
			t.Errorf("position %+v falls outside its own range %+v", pos, got.Range)
		}

		node := got

		for node.Parent != nil {
			if !rangeContains(node.Parent.Range, node.Range) {
				t.Errorf("chain for %+v breaks containment: %+v does not contain %+v",
					pos, node.Parent.Range, node.Range)

				break
			}

			node = *node.Parent
		}
	}
}
