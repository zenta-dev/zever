package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

// typeDefSrc exercises every typeDefinitionAt slot in one file. Line map
// (0-based) used by cursor positions below:
//
//	0: enum Status { active, dormant }
//	1: (blank)
//	2: entity User {
//	3: \tid: uuid @primary
//	4: \tstatus: Status
//	5: }
//	6: (blank)
//	7: message Greeting {
//	8: \ttext: string
//	9: }
//	10: (blank)
//	11: service Greeter {
//	12: \trpc Hello(req: Greeting) -> Greeting {
//	13: \t\tauth: none
//	14: \t}
//	15: }
//	16: (blank)
//	17: job Cleanup() {
//	18: \tqueue: default
//	19: }
//	20: (blank)
//	21: schedule Nightly {
//	22: \tcron: "0 0 * * *"
//	23: \tdispatch: Cleanup()
//	24: }
const typeDefSrc = `enum Status { active, dormant }

entity User {
	id: uuid @primary
	status: Status
}

message Greeting {
	text: string @validate(min_len: 1)
}

service Greeter {
	rpc Hello(req: Greeting) -> Greeting {
		auth: none
	}
}

job Cleanup() {
	queue: default
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: Cleanup()
}
`

// compileTypeDefFixture compiles the type-definition fixture and parses it.
func compileTypeDefFixture(t *testing.T) (*ir.Schema, *ast.File) {
	t.Helper()

	result, diags := compile.Compile(map[string]string{
		"main.zen": typeDefSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, fileDiags := parser.New("main.zen", []byte(typeDefSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics for main.zen: %v", fileDiags)
	}

	return result.Schema, file
}

// typeDefLoc unwraps a DefinitionResult union into its *Location arm, failing
// the test when the result holds any other arm. typeDefinitionAt only ever
// yields a *Location singleton or nil, so any other arm would mean the
// implementation invented a result.
func typeDefLoc(t *testing.T, res protocol.DefinitionResult) *protocol.Location {
	t.Helper()

	if res == nil {
		return nil
	}

	loc, ok := res.(*protocol.Location)
	if !ok {
		t.Fatalf("typeDefinitionAt() result = %T, want *Location", res)
	}

	return loc
}

// TestTypeDefinitionAtNamedEnumFieldType is the core enum case: the cursor
// sits on an entity field's named-enum type and must land on the enum's
// declaration line.
func TestTypeDefinitionAtNamedEnumFieldType(t *testing.T) {
	t.Parallel()

	schema, file := compileTypeDefFixture(t)

	cursor := cursorOn(t, typeDefSrc, 4, "status: Status")
	cursor.Character += uint32(len("status: "))

	loc := typeDefLoc(t, typeDefinitionAt(schema, file, cursor))
	if loc == nil {
		t.Fatal("typeDefinitionAt() = nil, want the enum Status location")
	}

	if !strings.HasSuffix(string(loc.URI), "/main.zen") {
		t.Errorf("URI = %q, want it to point at main.zen", loc.URI)
	}

	// `enum Status` is main.zen line 1 -> 0-based line 0, column 0.
	if loc.Range.Start.Line != 0 || loc.Range.Start.Character != 0 {
		t.Errorf("Range.Start = %+v, want 0:0", loc.Range.Start)
	}
}

// TestTypeDefinitionAtMessagePositions covers every message-typed slot: a
// param type and an rpc Returns type both land on the message's declaration.
func TestTypeDefinitionAtMessagePositions(t *testing.T) {
	t.Parallel()

	schema, file := compileTypeDefFixture(t)

	tests := []struct {
		name   string
		cursor protocol.Position
	}{
		{"param type", func() protocol.Position {
			c := cursorOn(t, typeDefSrc, 12, "req: Greeting")
			c.Character += uint32(len("req: "))
			return c
		}()},
		{"returns type", func() protocol.Position {
			c := cursorOn(t, typeDefSrc, 12, "-> Greeting")
			c.Character += uint32(len("-> "))
			return c
		}()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			loc := typeDefLoc(t, typeDefinitionAt(schema, file, tc.cursor))
			if loc == nil {
				t.Fatalf("typeDefinitionAt() = nil, want the message Greeting location")
			}

			// `message Greeting` is main.zen line 8 -> 0-based line 7.
			if loc.Range.Start.Line != 7 {
				t.Errorf("Range.Start.Line = %d, want 7 (message Greeting is line 8)", loc.Range.Start.Line)
			}
		})
	}
}

// TestTypeDefinitionAtNullPositions covers every cursor with no type
// definition: scalar field types have no declaration, a dispatch target
// names a job (a value, not a type), and unknown or empty positions name
// nothing at all.
func TestTypeDefinitionAtNullPositions(t *testing.T) {
	t.Parallel()

	schema, file := compileTypeDefFixture(t)

	tests := []struct {
		name   string
		cursor protocol.Position
	}{
		{"scalar entity field type", cursorOn(t, typeDefSrc, 3, "uuid")},
		{"scalar message field type", cursorOn(t, typeDefSrc, 8, "string")},
		{"dispatch job is not a type", cursorOn(t, typeDefSrc, 23, "Cleanup")},
		{"field name", cursorOn(t, typeDefSrc, 4, "status:")},
		{"empty line", protocol.Position{Line: 6, Character: 0}},
		{"far past end of file", protocol.Position{Line: 99, Character: 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := typeDefinitionAt(schema, file, tc.cursor); got != nil {
				t.Errorf("typeDefinitionAt() = %+v, want nil", got)
			}
		})
	}
}

// TestTypeDefinitionUnknownTargetReturnsNil pins the honest-null contract for
// a type the resolver could not match (mid-edit): no declaration, no jump.
func TestTypeDefinitionUnknownTargetReturnsNil(t *testing.T) {
	t.Parallel()

	src := "entity Task {\n\tkind: Ghost\n}\n"

	result, _ := compile.Compile(map[string]string{"a.zen": src})
	file, _ := parser.New("a.zen", []byte(src)).ParseFile()

	cursor := cursorOn(t, src, 1, "Ghost")

	if got := typeDefinitionAt(result.Schema, file, cursor); got != nil {
		t.Errorf("typeDefinitionAt() on an unresolved target = %+v, want nil", got)
	}
}

// TestTypeDefinitionHandler is the server-level round trip: the handler jumps
// an enum field type to the enum decl, returns null on a scalar, and returns
// null for an unknown document.
func TestTypeDefinitionHandler(t *testing.T) {
	t.Parallel()

	t.Run("enum field type resolves", func(t *testing.T) {
		t.Parallel()

		s := NewServer()
		openDoc(t, s, serverDocA, typeDefSrc)

		cursor := cursorOn(t, typeDefSrc, 4, "status: Status")
		cursor.Character += uint32(len("status: "))

		res, err := s.TypeDefinition(t.Context(), &protocol.TypeDefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     cursor,
			},
		})
		if err != nil {
			t.Fatalf("TypeDefinition: %v", err)
		}

		loc, ok := res.(*protocol.Location)
		if !ok || loc == nil {
			t.Fatalf("TypeDefinition result = %#v, want *Location", res)
		}

		if loc.Range.Start.Line != 0 {
			t.Errorf("Range.Start.Line = %d, want 0 (enum Status is line 1)", loc.Range.Start.Line)
		}
	})

	t.Run("scalar is null", func(t *testing.T) {
		t.Parallel()

		s := NewServer()
		openDoc(t, s, serverDocA, typeDefSrc)

		res, err := s.TypeDefinition(t.Context(), &protocol.TypeDefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverDocA)},
				Position:     cursorOn(t, typeDefSrc, 3, "uuid"),
			},
		})
		if err != nil {
			t.Fatalf("TypeDefinition: %v", err)
		}

		if res != nil {
			t.Errorf("TypeDefinition on a scalar type = %#v, want null", res)
		}
	})

	t.Run("unknown document is null", func(t *testing.T) {
		t.Parallel()

		s := NewServer()

		res, err := s.TypeDefinition(t.Context(), &protocol.TypeDefinitionParams{
			TextDocumentPositionParams: protocol.TextDocumentPositionParams{
				TextDocument: protocol.TextDocumentIdentifier{URI: uri.URI(serverUnknown)},
				Position:     protocol.Position{},
			},
		})
		if err != nil {
			t.Fatalf("TypeDefinition on an unknown document: %v", err)
		}

		if res != nil {
			t.Errorf("TypeDefinition() on an unknown document = %#v, want null", res)
		}
	})
}

// TestInitializeAdvertisesTypeDefinition guards the Initialize capability the
// client needs before it will ever send textDocument/typeDefinition.
func TestInitializeAdvertisesTypeDefinition(t *testing.T) {
	t.Parallel()

	s := NewServer()

	res, err := s.Initialize(t.Context(), &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	b, ok := res.Capabilities.TypeDefinitionProvider.(protocol.Boolean)
	if !ok || !bool(b) {
		t.Errorf("TypeDefinitionProvider = %#v, want Boolean(true)", res.Capabilities.TypeDefinitionProvider)
	}
}
