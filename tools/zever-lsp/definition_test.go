package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/compile"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

const (
	defASrc = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	defBSrc = "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: User @foreign_key(user_id)\n" +
		"}\n"
)

// cursorOn returns an LSP position pointing at the first character of needle
// on the given 0-based line of src, failing the test if it is not there.
func cursorOn(t *testing.T, src string, line int, needle string) protocol.Position {
	t.Helper()

	lines := strings.Split(src, "\n")
	if line >= len(lines) {
		t.Fatalf("line %d out of range in source with %d lines", line, len(lines))
	}

	col := strings.Index(lines[line], needle)
	if col < 0 {
		t.Fatalf("%q not found on line %d (%q)", needle, line, lines[line])
	}

	return protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture
}

// definitionLocs unwraps a DefinitionResult union into its LocationSlice arm,
// failing the test when the result holds any other arm.
func definitionLocs(t *testing.T, res protocol.DefinitionResult) protocol.LocationSlice {
	t.Helper()

	if res == nil {
		return nil
	}

	locs, ok := res.(protocol.LocationSlice)
	if !ok {
		if loc, isSingle := res.(*protocol.Location); isSingle && loc != nil {
			return protocol.LocationSlice{*loc}
		}

		t.Fatalf("definitionAt() result = %T, want LocationSlice or *Location", res)
	}

	return locs
}

// compileDefFixture compiles the two-file fixture and parses b.zen.
func compileDefFixture(t *testing.T) (*ir.Schema, *ast.File) {
	t.Helper()

	result, diags := compile.Compile(map[string]string{
		"a.zen": defASrc,
		"b.zen": defBSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, fileDiags := parser.New("b.zen", []byte(defBSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics for b.zen: %v", fileDiags)
	}

	return result.Schema, file
}

// TestDefinitionAtRelationTargetJumpsAcrossFiles is the core cross-file case:
// the cursor sits on `User` in b.zen and must land in a.zen.
func TestDefinitionAtRelationTargetJumpsAcrossFiles(t *testing.T) {
	schema, file := compileDefFixture(t)

	// Line 3 (0-based) is the `belongs_to user: User ...` line. Aim at the
	// target type, not the field name, by searching past "belongs_to user: ".
	line := 3
	col := strings.Index(strings.Split(defBSrc, "\n")[line], ": User") + len(": ")
	cursor := protocol.Position{Line: uint32(line), Character: uint32(col)} //nolint:gosec // test fixture

	locations := definitionLocs(t, definitionAt(schema, file, cursor))

	if len(locations) != 1 {
		t.Fatalf("got %d locations, want 1: %+v", len(locations), locations)
	}

	loc := locations[0]

	if !strings.HasSuffix(string(loc.URI), "/a.zen") {
		t.Errorf("URI = %q, want it to point at a.zen", loc.URI)
	}

	if !strings.HasPrefix(string(loc.URI), "file://") {
		t.Errorf("URI = %q, want a file:// URI", loc.URI)
	}

	// `entity User` is on the first line of a.zen -> 0-based line 0.
	if loc.Range.Start.Line != 0 {
		t.Errorf("Range.Start.Line = %d, want 0 (entity User is a.zen line 1)", loc.Range.Start.Line)
	}

	if loc.Range.Start.Character != 0 {
		t.Errorf("Range.Start.Character = %d, want 0", loc.Range.Start.Character)
	}
}

// TestDefinitionAtNonTargetReturnsNil covers every position that has nothing
// to jump to: a scalar field type, a field name, whitespace, the relation's
// own field name.
func TestDefinitionAtNonTargetReturnsNil(t *testing.T) {
	schema, file := compileDefFixture(t)

	tests := []struct {
		name   string
		cursor protocol.Position
	}{
		{"scalar field type", cursorOn(t, defBSrc, 2, "uuid")},
		{"field name", cursorOn(t, defBSrc, 1, "id")},
		{"relation field name", cursorOn(t, defBSrc, 3, "user:")},
		{"empty line", protocol.Position{Line: 4, Character: 0}},
		{"far past end of file", protocol.Position{Line: 99, Character: 0}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := definitionAt(schema, file, tc.cursor); got != nil {
				t.Errorf("definitionAt() = %+v, want nil", got)
			}
		})
	}
}

func TestDefinitionUnknownTargetReturnsNil(t *testing.T) {
	src := "entity Task {\n\tuser_id: uuid\n\tbelongs_to user: Ghost @foreign_key(user_id)\n}\n"

	result, _ := compile.Compile(map[string]string{"b.zen": src})
	file, _ := parser.New("b.zen", []byte(src)).ParseFile()

	cursor := cursorOn(t, src, 2, "Ghost")

	if got := definitionAt(result.Schema, file, cursor); got != nil {
		t.Errorf("definitionAt() on an unresolved target = %+v, want nil", got)
	}
}

const (
	defMsgASrc = "message Greeting {\n" +
		"\ttext: string @validate(min_len: 1)\n" +
		"}\n"

	defMsgBSrc = "service Greeter {\n" +
		"\trpc Hello(req: Greeting) -> Greeting {\n" +
		"\t\tauth: none\n" +
		"\t}\n" +
		"}\n"
)

// compileMsgDefFixture compiles a two-file fixture where a service in one
// file references a message declared in the other, both as an rpc param
// type and as its Returns type -- covering the gaps definitionAt used to
// have: only a relation target was a jump target, so a message used as a
// param or Returns type (the whole point of the message construct) had
// nowhere to jump to, cross-file or otherwise.
func compileMsgDefFixture(t *testing.T) (*ir.Schema, *ast.File) {
	t.Helper()

	result, diags := compile.Compile(map[string]string{
		"a.zen": defMsgASrc,
		"b.zen": defMsgBSrc,
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, fileDiags := parser.New("b.zen", []byte(defMsgBSrc)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics for b.zen: %v", fileDiags)
	}

	return result.Schema, file
}

// TestDefinitionAtRPCReturnsJumpsToMessageAcrossFiles: cursor on the
// Returns type name lands on the message's declaration in the other file.
func TestDefinitionAtRPCReturnsJumpsToMessageAcrossFiles(t *testing.T) {
	schema, file := compileMsgDefFixture(t)

	cursor := cursorOn(t, defMsgBSrc, 1, "-> Greeting")
	cursor.Character += uint32(len("-> "))

	locations := definitionLocs(t, definitionAt(schema, file, cursor))
	if len(locations) != 1 {
		t.Fatalf("got %d locations, want 1: %+v", len(locations), locations)
	}

	if loc := locations[0]; !strings.HasSuffix(string(loc.URI), "/a.zen") || loc.Range.Start.Line != 0 {
		t.Errorf("locations[0] = %+v, want a.zen line 0", loc)
	}
}

// TestDefinitionAtParamTypeJumpsToMessageAcrossFiles: cursor on a param's
// type name (the message-as-param shape, e.g. `rpc Hello(req: Greeting)`)
// lands on the message's declaration in the other file.
func TestDefinitionAtParamTypeJumpsToMessageAcrossFiles(t *testing.T) {
	schema, file := compileMsgDefFixture(t)

	cursor := cursorOn(t, defMsgBSrc, 1, "req: Greeting")
	cursor.Character += uint32(len("req: "))

	locations := definitionLocs(t, definitionAt(schema, file, cursor))
	if len(locations) != 1 {
		t.Fatalf("got %d locations, want 1: %+v", len(locations), locations)
	}

	if loc := locations[0]; !strings.HasSuffix(string(loc.URI), "/a.zen") || loc.Range.Start.Line != 0 {
		t.Errorf("locations[0] = %+v, want a.zen line 0", loc)
	}
}

// TestDefinitionAtMessageFieldRefJumpsToMessage: a message field that
// itself references another message by name (Field.Ref) is a jump target
// too, same as a param/Returns reference.
func TestDefinitionAtMessageFieldRefJumpsToMessage(t *testing.T) {
	src := "message Inner {\n" +
		"\tvalue: string\n" +
		"}\n\n" +
		"message Outer {\n" +
		"\tinner: Inner\n" +
		"}\n"

	result, diags := compile.Compile(map[string]string{"a.zen": src})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	file, fileDiags := parser.New("a.zen", []byte(src)).ParseFile()
	if fileDiags.HasErrors() {
		t.Fatalf("unexpected parse diagnostics: %v", fileDiags)
	}

	cursor := cursorOn(t, src, 5, "Inner")

	locations := definitionLocs(t, definitionAt(result.Schema, file, cursor))
	if len(locations) != 1 {
		t.Fatalf("got %d locations, want 1: %+v", len(locations), locations)
	}

	if loc := locations[0]; loc.Range.Start.Line != 0 {
		t.Errorf("locations[0] = %+v, want line 0 (message Inner)", loc)
	}
}

func TestFindMessage(t *testing.T) {
	schema, _ := compileMsgDefFixture(t)

	if got := findMessage(schema, "Greeting"); got == nil || got.Name != "Greeting" {
		t.Errorf("findMessage(Greeting) = %v, want the Greeting message", got)
	}

	if got := findMessage(schema, "Nope"); got != nil {
		t.Errorf("findMessage(Nope) = %v, want nil", got)
	}

	if got := findMessage(nil, "Greeting"); got != nil {
		t.Errorf("findMessage on a nil schema = %v, want nil", got)
	}
}

func TestFindEntity(t *testing.T) {
	schema, _ := compileDefFixture(t)

	if got := findEntity(schema, "User"); got == nil || got.Name != "User" {
		t.Errorf("findEntity(User) = %v, want the User entity", got)
	}

	if got := findEntity(schema, "Nope"); got != nil {
		t.Errorf("findEntity(Nope) = %v, want nil", got)
	}

	if got := findEntity(nil, "User"); got != nil {
		t.Errorf("findEntity on a nil schema = %v, want nil", got)
	}
}

func TestAllEntityNames_returnsEntitiesInDeclarationOrder(t *testing.T) {
	schema, _ := compileDefFixture(t)

	got := allEntityNames(schema)

	if len(got) != 2 {
		t.Fatalf("allEntityNames() = %d entities, want 2: %v", len(got), got)
	}

	if got[0].Name != "User" || got[1].Name != "Task" {
		t.Errorf("allEntityNames() = [%s %s], want [User Task]", got[0].Name, got[1].Name)
	}

	if allEntityNames(nil) != nil {
		t.Errorf("allEntityNames(nil) = %v, want nil", allEntityNames(nil))
	}
}
