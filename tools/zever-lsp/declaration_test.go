package main

import (
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// declarationLoc unwraps a DeclarationResult union into its *Location arm,
// failing the test when the result holds any other arm. definitionAt only
// ever yields a *Location singleton or nil, and Declaration forwards it
// unchanged, so any other arm would mean the alias invented a result.
func declarationLoc(t *testing.T, res protocol.DeclarationResult) *protocol.Location {
	t.Helper()

	if res == nil {
		return nil
	}

	loc, ok := res.(*protocol.Location)
	if !ok {
		t.Fatalf("Declaration() result = %T, want *Location", res)
	}

	return loc
}

// TestDeclarationMirrorsDefinition is the core alias case: the cursor sits
// on `User` in serverValidDoc's belongs_to line and must land on
// `entity User` at line 0, exactly where Definition jumps.
func TestDeclarationMirrorsDefinition(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	cursor := cursorOn(t, serverValidDoc, 10, ": User")
	cursor.Character += uint32(len(": "))

	got, err := s.Declaration(t.Context(), &protocol.DeclarationParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursor,
		},
	})
	if err != nil {
		t.Fatalf("Declaration: %v", err)
	}

	loc := declarationLoc(t, got)
	if loc == nil {
		t.Fatal("Declaration() = nil, want the entity User location")
	}

	if loc.Range.Start.Line != 0 {
		t.Errorf("Range.Start.Line = %d, want 0 (entity User is line 1)", loc.Range.Start.Line)
	}

	def, err := s.Definition(t.Context(), &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursor,
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}

	defLoc, ok := def.(*protocol.Location)
	if !ok || defLoc == nil {
		t.Fatalf("Definition() = %#v, want a location to compare against", def)
	}

	if *loc != *defLoc {
		t.Errorf("Declaration() = %+v, want Definition()'s %+v", loc, defLoc)
	}
}

// TestDeclarationReturnsNullWhenNothingJumpable covers every position with
// no declaration to jump to: a scalar field type and an unknown document
// both yield nil, same as Definition.
func TestDeclarationReturnsNullWhenNothingJumpable(t *testing.T) {
	t.Parallel()

	s := NewServer()
	openDoc(t, s, serverDocA, serverValidDoc)

	got, err := s.Declaration(t.Context(), &protocol.DeclarationParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
			Position:     cursorOn(t, serverValidDoc, 1, "uuid"),
		},
	})
	if err != nil {
		t.Fatalf("Declaration: %v", err)
	}

	if got != nil {
		t.Errorf("Declaration() on a scalar type = %+v, want nil", got)
	}

	got, err = s.Declaration(t.Context(), &protocol.DeclarationParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
			Position:     protocol.Position{},
		},
	})
	if err != nil {
		t.Fatalf("Declaration on an unknown document: %v", err)
	}

	if got != nil {
		t.Errorf("Declaration() on an unknown document = %+v, want nil", got)
	}
}

// TestInitializeAdvertisesDeclaration guards the Initialize capability the
// client needs before it will ever send textDocument/declaration.
func TestInitializeAdvertisesDeclaration(t *testing.T) {
	t.Parallel()

	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	b, ok := res.Capabilities.DeclarationProvider.(protocol.Boolean)
	if !ok || !bool(b) {
		t.Errorf("DeclarationProvider = %#v, want Boolean(true)", res.Capabilities.DeclarationProvider)
	}
}
