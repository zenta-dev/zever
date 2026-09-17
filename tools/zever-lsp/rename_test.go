package main

import (
	"errors"
	"strings"
	"testing"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

const (
	renameUserSrc = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\towner_id: uuid\n" +
		"}\n"

	renameTaskSrc = "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: User @foreign_key(user_id)\n" +
		"}\n"

	renameServiceSrc = "service UserService {\n" +
		"\trpc GetUser(id: uuid) -> User {\n" +
		"\t\thttp: GET \"/users/{id}\"\n" +
		"\t\tauth: required\n" +
		"\t}\n" +
		"}\n"

	renamePermissionSrc = "service AdminService {\n" +
		"\trpc ShowUser(id: uuid) -> Task {\n" +
		"\t\tpermission: check(\"can_read\", resource: User, owner_field: owner_id)\n" +
		"\t}\n" +
		"}\n"

	renameFieldUserSrc = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"\tindex(email)\n" +
		"}\n"

	renameFieldOwnerSrc = "service AdminService {\n" +
		"\trpc ShowUser(id: uuid) -> User {\n" +
		"\t\tpermission: check(\"can_read\", resource: User, owner_field: email)\n" +
		"\t}\n" +
		"}\n"

	renameFieldOrderSrc = "entity Order {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"}\n"

	renameJobSrc = "job CleanupJob(id: uuid) {\n" +
		"\tqueue: \"default\"\n" +
		"}\n"

	renameScheduleSrc = "schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: CleanupJob(id: \"x\")\n" +
		"}\n"

	renameRPCAdminSrc = "service AdminService {\n" +
		"\trpc Get(id: uuid) -> User {\n" +
		"\t\tauth: required\n" +
		"\t}\n" +
		"}\n"

	renameRPCReportSrc = "service ReportService {\n" +
		"\trpc Get(id: uuid) -> User {\n" +
		"\t\tauth: required\n" +
		"\t}\n" +
		"}\n"
)

// parseRenameFiles parses every fixture source in memory, keyed by file name,
// the way Workspace.ParsedFiles feeds the rename builders: one *ast.File per
// path. No filesystem or network access.
func parseRenameFiles(t *testing.T, files map[string]string) map[string]*ast.File {
	t.Helper()

	parsed := make(map[string]*ast.File, len(files))

	for path, src := range files {
		file, _ := parser.New(path, []byte(src)).ParseFile()
		if file == nil {
			t.Fatalf("failed to parse fixture %s:\n%s", path, src)
		}

		parsed[path] = file
	}

	return parsed
}

// renameEditsFor returns the edits a WorkspaceEdit carries for one file,
// failing when the edit itself is nil.
func renameEditsFor(t *testing.T, edit *protocol.WorkspaceEdit, id uri.URI) []protocol.TextEdit {
	t.Helper()

	if edit == nil {
		t.Fatalf("WorkspaceEdit is nil")
	}

	return edit.Changes[id]
}

// renameTargetMustResolve resolves cursor in file and fails when it does not
// point at the expected kind/name/owner.
func renameTargetMustResolve(
	t *testing.T, file *ast.File, cursor protocol.Position, kind symbolKind, name, owner string,
) renameTarget {
	t.Helper()

	target, ok := renameTargetAt(file, cursor)
	if !ok {
		t.Fatalf("renameTargetAt() = not found, want %s %q", kind, name)
	}

	if target.Kind != kind || target.Name != name || target.Owner != owner {
		t.Fatalf("renameTargetAt() = %+v, want kind %s name %q owner %q", target, kind, name, owner)
	}

	return target
}

// TestRenameEntityEdits_multiFile_updatesDeclarationAndTargets is the core
// multi-file case: renaming from the declaration must also rewrite the
// relation target in a file the request never mentions.
func TestRenameEntityEdits_multiFile_updatesDeclarationAndTargets(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"b.zen": renameTaskSrc,
	})

	renameTargetMustResolve(t, parsed["a.zen"], cursorOn(t, renameUserSrc, 0, "User"), entityKind, "User", "")

	edit, err := renameEntityEdits(parsed, "User", "Person")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	declEdits := renameEditsFor(t, edit, pathToURI("a.zen"))
	if len(declEdits) != 1 {
		t.Fatalf("a.zen edits = %d, want 1: %+v", len(declEdits), declEdits)
	}

	if declEdits[0].NewText != "Person" {
		t.Errorf("a.zen NewText = %q, want %q", declEdits[0].NewText, "Person")
	}

	// `entity User` is on line 0, with "User" starting at column 7.
	wantDecl := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 7},
		End:   protocol.Position{Line: 0, Character: 11},
	}
	if declEdits[0].Range != wantDecl {
		t.Errorf("a.zen Range = %+v, want %+v (exactly the `User` identifier)", declEdits[0].Range, wantDecl)
	}

	relEdits := renameEditsFor(t, edit, pathToURI("b.zen"))
	if len(relEdits) != 1 {
		t.Fatalf("b.zen edits = %d, want 1: %+v", len(relEdits), relEdits)
	}

	if relEdits[0].NewText != "Person" {
		t.Errorf("b.zen NewText = %q, want %q", relEdits[0].NewText, "Person")
	}

	if relEdits[0].Range.Start.Line != 3 {
		t.Errorf("b.zen edit line = %d, want 3 (the belongs_to line)", relEdits[0].Range.Start.Line)
	}
}

// TestRenameEntityEdits_rpcReturnType_isRewritten proves an rpc's `-> User`
// return type is rewritten too, even though it lives in a third file.
func TestRenameEntityEdits_rpcReturnType_isRewritten(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"b.zen": renameTaskSrc,
		"c.zen": renameServiceSrc,
	})

	edit, err := renameEntityEdits(parsed, "User", "Person")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	rpcEdits := renameEditsFor(t, edit, pathToURI("c.zen"))
	if len(rpcEdits) != 1 {
		t.Fatalf("c.zen edits = %d, want 1 (the rpc return type): %+v", len(rpcEdits), rpcEdits)
	}

	if rpcEdits[0].NewText != "Person" {
		t.Errorf("c.zen NewText = %q, want %q", rpcEdits[0].NewText, "Person")
	}

	// `rpc GetUser(id: uuid) -> User {` is line 1 of the service source.
	if rpcEdits[0].Range.Start.Line != 1 {
		t.Errorf("c.zen edit line = %d, want 1 (the rpc signature line)", rpcEdits[0].Range.Start.Line)
	}

	wantCol := uint32(strings.Index(strings.Split(renameServiceSrc, "\n")[1], "-> User") + len("-> ")) //nolint:gosec // test fixture
	if rpcEdits[0].Range.Start.Character != wantCol {
		t.Errorf("c.zen edit column = %d, want %d (the return type ident)", rpcEdits[0].Range.Start.Character, wantCol)
	}
}

// TestRenameEntityEdits_permissionResource_isRewritten covers the fourth
// reference kind: `permission: check(..., resource: User, ...)`.
func TestRenameEntityEdits_permissionResource_isRewritten(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"b.zen": renameTaskSrc,
		"d.zen": renamePermissionSrc,
	})

	edit, err := renameEntityEdits(parsed, "User", "Person")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	// The rpc returns Task, so User appears exactly once in d.zen: as the
	// permission resource.
	permEdits := renameEditsFor(t, edit, pathToURI("d.zen"))
	if len(permEdits) != 1 {
		t.Fatalf("d.zen edits = %d, want 1 (the permission resource): %+v", len(permEdits), permEdits)
	}

	if permEdits[0].NewText != "Person" {
		t.Errorf("d.zen NewText = %q, want %q", permEdits[0].NewText, "Person")
	}

	permLine := strings.Split(renamePermissionSrc, "\n")[2]
	wantCol := uint32(strings.Index(permLine, "resource: User") + len("resource: ")) //nolint:gosec // test fixture

	if permEdits[0].Range.Start.Line != 2 || permEdits[0].Range.Start.Character != wantCol {
		t.Errorf("d.zen edit start = %+v, want line 2 char %d (the resource ident)",
			permEdits[0].Range.Start, wantCol)
	}

	if permEdits[0].Range.End.Character != wantCol+uint32(len("User")) {
		t.Errorf("d.zen edit end char = %d, want %d", permEdits[0].Range.End.Character, wantCol+4)
	}
}

// TestRenameTargetAt_relationFieldName_notFound proves a cursor on a
// relation's own field name -- which has no identity independent of the
// relation declaring it -- resolves to nothing instead of an edit a client
// would show as a successful no-op.
func TestRenameTargetAt_relationFieldName_notFound(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"b.zen": renameTaskSrc,
	})

	// `belongs_to user: User @foreign_key(user_id)` -- "user" here is the
	// relation's own field name, not the User entity.
	if target, ok := renameTargetAt(parsed["b.zen"], cursorOn(t, renameTaskSrc, 3, "user:")); ok {
		t.Errorf("renameTargetAt() = %+v, want not found", target)
	}

	if _, err := renameSymbolEdits(parsed, "", "owner", entityRefsIn); !errors.Is(err, errRenameUnsupported) {
		t.Errorf("renameSymbolEdits() error = %v, want errRenameUnsupported", err)
	}
}

// TestRenameTargetAt_entityDeclaration_resolvesRange is the prepareRename
// equivalent: the resolved target's span exactly covers the identifier, so
// the server shell can pre-fill the client's rename prompt from it.
func TestRenameTargetAt_entityDeclaration_resolvesRange(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	target := renameTargetMustResolve(t, parsed["a.zen"], cursorOn(t, renameUserSrc, 0, "User"), entityKind, "User", "")

	want := protocol.Range{
		Start: protocol.Position{Line: 0, Character: 7},
		End:   protocol.Position{Line: 0, Character: 11},
	}

	if got := identRange(target.Pos, target.Name); got != want {
		t.Errorf("identRange() = %+v, want %+v (exactly `User`)", got, want)
	}
}

// TestRenameTargetAt_unsupportedCursors_notFound proves renameTargetAt
// returns not-found (not an error) for positions with nothing renameable:
// whitespace, past EOF, an rpc parameter name, and a relation's own field
// name.
func TestRenameTargetAt_unsupportedCursors_notFound(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"b.zen": renameTaskSrc,
		"c.zen": renameServiceSrc,
	})

	tests := []struct {
		name   string
		file   *ast.File
		cursor protocol.Position
	}{
		{"whitespace after the brace", parsed["a.zen"], protocol.Position{Line: 3, Character: 0}},
		{"past the end of the file", parsed["a.zen"], protocol.Position{Line: 99, Character: 0}},
		{"rpc param name", parsed["c.zen"], cursorOn(t, renameServiceSrc, 1, "id: uuid")},
		{"relation field name", parsed["b.zen"], cursorOn(t, renameTaskSrc, 3, "user:")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if target, ok := renameTargetAt(tc.file, tc.cursor); ok {
				t.Errorf("renameTargetAt() = %+v, want not found", target)
			}
		})
	}
}

// TestRenameTargetAt_rpcReturnCursor_resolvesEntity proves the cursor may
// sit on a reference rather than the declaration and still resolve the
// whole set.
func TestRenameTargetAt_rpcReturnCursor_resolvesEntity(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"c.zen": renameServiceSrc,
	})

	cursor := cursorOn(t, renameServiceSrc, 1, "-> User")
	cursor.Character += uint32(len("-> "))

	target, ok := renameTargetAt(parsed["c.zen"], cursor)
	if !ok {
		t.Fatalf("renameTargetAt() on the rpc return type = not found, want the User entity")
	}

	if target.Kind != entityKind || target.Name != "User" {
		t.Errorf("renameTargetAt() = %+v, want entity User", target)
	}

	edit, err := renameEntityEdits(parsed, target.Name, "Person")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	if got := len(renameEditsFor(t, edit, pathToURI("a.zen"))); got != 1 {
		t.Errorf("a.zen edits = %d, want 1 (the declaration)", got)
	}

	if got := len(renameEditsFor(t, edit, pathToURI("c.zen"))); got != 1 {
		t.Errorf("c.zen edits = %d, want 1 (the return type)", got)
	}
}

// TestRenameSymbolEdits_emptyOldName_returnsUnsupported pins the guard: an
// empty old name can never resolve to a symbol.
func TestRenameSymbolEdits_emptyOldName_returnsUnsupported(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	edit, err := renameEntityEdits(parsed, "", "Person")
	if !errors.Is(err, errRenameUnsupported) {
		t.Fatalf("renameEntityEdits() error = %v, want errRenameUnsupported", err)
	}

	if edit != nil {
		t.Errorf("renameEntityEdits() edit = %+v, want nil alongside the error", edit)
	}
}

// TestRenameEntityEdits_emptyNewName_returnsError proves a blank new name is
// rejected before any edit is built.
func TestRenameEntityEdits_emptyNewName_returnsError(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	_, err := renameEntityEdits(parsed, "User", "")
	if !errors.Is(err, errRenameEmptyName) {
		t.Fatalf("renameEntityEdits() error = %v, want errRenameEmptyName", err)
	}
}

// TestIsValidZenIdent_table mirrors the lexer's isIdentStart/isIdentContinue
// rule: '_' or an ASCII letter to start, '_'/letter/digit to continue.
func TestIsValidZenIdent_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ok   bool
	}{
		{"Person", true},
		{"_private", true},
		{"camelCase", true},
		{"snake_case", true},
		{"a1", true},
		{"_", true},
		{"", false},
		{"1Person", false},
		{"9lives", false},
		{"has-hyphen", false},
		{"has space", false},
		{"has.dot", false},
		{"emoji😀", false},
		{"éclair", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isValidZenIdent(tc.name); got != tc.ok {
				t.Errorf("isValidZenIdent(%q) = %v, want %v", tc.name, got, tc.ok)
			}
		})
	}
}

// TestRenameEntityEdits_invalidNewName_returnsError proves representative
// malformed names are rejected with an error and produce no edits.
func TestRenameEntityEdits_invalidNewName_returnsError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newName string
	}{
		{"starts with digit", "1Person"},
		{"contains hyphen", "Per-son"},
		{"contains space", "Per son"},
		{"empty string", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

			edit, err := renameEntityEdits(parsed, "User", tc.newName)
			if err == nil {
				t.Fatalf("renameEntityEdits(%q) = nil error, want an error", tc.newName)
			}

			if edit != nil {
				t.Errorf("renameEntityEdits(%q) = %+v, want nil edit", tc.newName, edit)
			}
		})
	}
}

// TestRenameEntityEdits_validNewName_producesEdit confirms new-name
// validation leaves the successful path unaffected.
func TestRenameEntityEdits_validNewName_producesEdit(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	edit, err := renameEntityEdits(parsed, "User", "Person")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	edits := renameEditsFor(t, edit, pathToURI("a.zen"))
	if len(edits) != 1 {
		t.Fatalf("edits = %d, want 1: %+v", len(edits), edits)
	}

	if edits[0].NewText != "Person" {
		t.Errorf("NewText = %q, want %q", edits[0].NewText, "Person")
	}
}

// TestRenameFieldEdits_scoped_updatesDeclIndexAndOwnerField is the
// field-rename core case: renaming from the field's own declaration must
// also rewrite its index column (same file) and the owner_field reference
// naming it in another service's permission block, while leaving a
// same-named field on an unrelated entity untouched -- proving field rename
// is entity-scoped, not name-scoped.
func TestRenameFieldEdits_scoped_updatesDeclIndexAndOwnerField(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameFieldUserSrc,
		"d.zen": renameFieldOwnerSrc,
		"e.zen": renameFieldOrderSrc,
	})

	target := renameTargetMustResolve(t, parsed["a.zen"], cursorOn(t, renameFieldUserSrc, 2, "email"), fieldKind, "email", "User")

	edit, err := renameFieldEdits(parsed, target.Owner, target.Name, "email_address")
	if err != nil {
		t.Fatalf("renameFieldEdits() error = %v, want nil", err)
	}

	declEdits := renameEditsFor(t, edit, pathToURI("a.zen"))
	if len(declEdits) != 2 {
		t.Fatalf("a.zen edits = %d, want 2 (declaration + index column): %+v", len(declEdits), declEdits)
	}

	for _, e := range declEdits {
		if e.NewText != "email_address" {
			t.Errorf("a.zen NewText = %q, want %q", e.NewText, "email_address")
		}
	}

	// Declaration (line 2) sorts before the index column (line 3).
	if declEdits[0].Range.Start.Line != 2 || declEdits[1].Range.Start.Line != 3 {
		t.Errorf("a.zen edit lines = %d,%d, want 2,3 (sorted)", declEdits[0].Range.Start.Line, declEdits[1].Range.Start.Line)
	}

	ownerEdits := renameEditsFor(t, edit, pathToURI("d.zen"))
	if len(ownerEdits) != 1 {
		t.Fatalf("d.zen edits = %d, want 1 (the owner_field reference): %+v", len(ownerEdits), ownerEdits)
	}

	if ownerEdits[0].NewText != "email_address" {
		t.Errorf("d.zen NewText = %q, want %q", ownerEdits[0].NewText, "email_address")
	}

	if orderEdits := renameEditsFor(t, edit, pathToURI("e.zen")); len(orderEdits) != 0 {
		t.Errorf("e.zen edits = %d, want 0 (Order.email is a different entity's field)", len(orderEdits))
	}
}

// TestRenameJobEdits_multiFile_updatesDeclAndDispatch proves a job rename
// reaches every schedule dispatching it, across files.
func TestRenameJobEdits_multiFile_updatesDeclAndDispatch(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameJobSrc,
		"b.zen": renameScheduleSrc,
	})

	renameTargetMustResolve(t, parsed["a.zen"], cursorOn(t, renameJobSrc, 0, "CleanupJob"), jobKind, "CleanupJob", "")

	edit, err := renameJobEdits(parsed, "CleanupJob", "PurgeJob")
	if err != nil {
		t.Fatalf("renameJobEdits() error = %v, want nil", err)
	}

	declEdits := renameEditsFor(t, edit, pathToURI("a.zen"))
	if len(declEdits) != 1 || declEdits[0].NewText != "PurgeJob" {
		t.Errorf("a.zen edits = %+v, want 1 edit renaming to PurgeJob", declEdits)
	}

	dispatchEdits := renameEditsFor(t, edit, pathToURI("b.zen"))
	if len(dispatchEdits) != 1 || dispatchEdits[0].NewText != "PurgeJob" {
		t.Errorf("b.zen edits = %+v, want 1 edit renaming the dispatch target to PurgeJob", dispatchEdits)
	}
}

// TestRenameServiceEdits_singleFile_onlyDeclaration proves a service rename
// does not walk the whole workspace: nothing else in the resolver looks up
// a service by name, so the only edit is the declaration itself.
func TestRenameServiceEdits_singleFile_onlyDeclaration(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"c.zen": renameServiceSrc})

	renameTargetMustResolve(t, parsed["c.zen"], cursorOn(t, renameServiceSrc, 0, "UserService"), serviceKind, "UserService", "")

	edit, err := renameServiceEdits(parsed, "UserService", "AccountService")
	if err != nil {
		t.Fatalf("renameServiceEdits() error = %v, want nil", err)
	}

	if len(edit.Changes) != 1 {
		t.Fatalf("WorkspaceEdit touches %d files, want exactly 1 (no spurious workspace walk): %+v",
			len(edit.Changes), edit.Changes)
	}

	edits := renameEditsFor(t, edit, pathToURI("c.zen"))
	if len(edits) != 1 || edits[0].NewText != "AccountService" {
		t.Errorf("c.zen edits = %+v, want exactly 1 edit renaming to AccountService", edits)
	}
}

// TestRenameRPCEdits_serviceScoped_onlyOwnService proves two services
// declaring a same-named rpc in different files do not cross-contaminate.
func TestRenameRPCEdits_serviceScoped_onlyOwnService(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"c.zen": renameRPCAdminSrc,
		"e.zen": renameRPCReportSrc,
	})

	target := renameTargetMustResolve(t, parsed["c.zen"], cursorOn(t, renameRPCAdminSrc, 1, "Get"), rpcKind, "Get", "AdminService")

	edit, err := renameRPCEdits(parsed, target.Owner, target.Name, "Fetch")
	if err != nil {
		t.Fatalf("renameRPCEdits() error = %v, want nil", err)
	}

	adminEdits := renameEditsFor(t, edit, pathToURI("c.zen"))
	if len(adminEdits) != 1 || adminEdits[0].NewText != "Fetch" {
		t.Errorf("c.zen edits = %+v, want 1 edit renaming to Fetch", adminEdits)
	}

	if reportEdits := renameEditsFor(t, edit, pathToURI("e.zen")); len(reportEdits) != 0 {
		t.Errorf("e.zen edits = %d, want 0 (ReportService.Get is a different service's rpc)", len(reportEdits))
	}
}

// TestRenameTargetAt_eachKind_resolves documents the full dispatch order:
// every renameable kind resolves from its own cursor position.
func TestRenameTargetAt_eachKind_resolves(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameFieldUserSrc,
		"b.zen": renameTaskSrc,
		"c.zen": renameServiceSrc,
		"d.zen": renameFieldOwnerSrc,
		"j.zen": renameJobSrc,
		"s.zen": renameScheduleSrc,
	})

	tests := []struct {
		name          string
		file          string
		src           string
		line          int
		needle        string
		wantKind      symbolKind
		wantName      string
		wantOwner     string
		nudgePastRune string
	}{
		{"entity declaration", "a.zen", renameFieldUserSrc, 0, "User", entityKind, "User", "", ""},
		{"relation target", "b.zen", renameTaskSrc, 3, ": User", entityKind, "User", "", ": "},
		{"field declaration", "a.zen", renameFieldUserSrc, 2, "email", fieldKind, "email", "User", ""},
		{"field type", "a.zen", renameFieldUserSrc, 2, "string", fieldKind, "email", "User", ""},
		{"index column", "a.zen", renameFieldUserSrc, 3, "email", fieldKind, "email", "User", ""},
		{"owner_field reference", "d.zen", renameFieldOwnerSrc, 2, "owner_field: email", fieldKind, "email", "User", "owner_field: "},
		{"job declaration", "j.zen", renameJobSrc, 0, "CleanupJob", jobKind, "CleanupJob", "", ""},
		{"dispatch target", "s.zen", renameScheduleSrc, 2, "CleanupJob", jobKind, "CleanupJob", "", ""},
		{"service declaration", "c.zen", renameServiceSrc, 0, "UserService", serviceKind, "UserService", "", ""},
		{"rpc declaration", "c.zen", renameServiceSrc, 1, "GetUser", rpcKind, "GetUser", "UserService", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cursor := cursorOn(t, tc.src, tc.line, tc.needle)
			if tc.nudgePastRune != "" {
				cursor.Character += uint32(len(tc.nudgePastRune)) //nolint:gosec // test fixture
			}

			renameTargetMustResolve(t, parsed[tc.file], cursor, tc.wantKind, tc.wantName, tc.wantOwner)
		})
	}
}

// TestRefsAt_hitAndMiss_pinsSharedMatcher covers the cursor matcher every
// per-kind walk funnels through.
func TestRefsAt_hitAndMiss_pinsSharedMatcher(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	refs := entityRefsIn(parsed["a.zen"])
	if len(refs) == 0 {
		t.Fatalf("entityRefsIn() = empty, want the User declaration")
	}

	if got, ok := refsAt(refs, cursorOn(t, renameUserSrc, 0, "User")); !ok || got.Name != "User" {
		t.Errorf("refsAt() = %+v,%v, want the User ref", got, ok)
	}

	if got, ok := refsAt(refs, protocol.Position{Line: 99, Character: 0}); ok {
		t.Errorf("refsAt() = %+v,true, want not found", got)
	}

	if got, ok := refsAt(nil, cursorOn(t, renameUserSrc, 0, "User")); ok {
		t.Errorf("refsAt(nil) = %+v,true, want not found", got)
	}
}

// TestPermissionCallArgRef_malformedYieldsNothing walks every malformed
// permission shape: each must yield ok == false rather than a guess.
func TestPermissionCallArgRef_malformedYieldsNothing(t *testing.T) {
	t.Parallel()

	pos := diag.Position{File: "a.zen", Line: 1, Col: 1}

	tests := []struct {
		name string
		rpc  *ast.RPCDecl
	}{
		{"nil rpc", nil},
		{"nil permission", &ast.RPCDecl{}},
		{"non-call permission", &ast.RPCDecl{Permission: &ast.IdentValue{Name: "check", Pos: pos}}},
		{"wrong call name", &ast.RPCDecl{Permission: &ast.CallValue{Name: "allow", Args: nil}}},
		{
			"nil arg skipped",
			&ast.RPCDecl{Permission: &ast.CallValue{Name: "check", Args: []*ast.Arg{nil}}},
		},
		{
			"arg name mismatch",
			&ast.RPCDecl{Permission: &ast.CallValue{Name: "check", Args: []*ast.Arg{
				{Pos: pos, Name: "other", Value: &ast.IdentValue{Name: "User", Pos: pos}},
			}}},
		},
		{
			"non-ident value skipped",
			&ast.RPCDecl{Permission: &ast.CallValue{Name: "check", Args: []*ast.Arg{
				{Pos: pos, Name: "resource", Value: &ast.CallValue{Name: "x"}},
			}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if ref, ok := permissionCallArgRef(tc.rpc, "resource"); ok {
				t.Errorf("permissionCallArgRef() = %+v,true, want not found", ref)
			}

			if _, ok := permissionResourceRef(tc.rpc); ok {
				t.Errorf("permissionResourceRef() = true, want false")
			}

			if _, ok := ownerFieldRef(tc.rpc); ok {
				t.Errorf("ownerFieldRef() = true, want false")
			}
		})
	}
}

// TestPermissionCallArgRef_wellFormed_extractsBothArgs proves the shared
// extractor serves both the resource and the owner_field argument.
func TestPermissionCallArgRef_wellFormed_extractsBothArgs(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"d.zen": renamePermissionSrc})

	var rpc *ast.RPCDecl

	for _, decl := range flattenDecls(parsed["d.zen"]) {
		if svc, ok := decl.(*ast.ServiceDecl); ok && len(svc.RPCs) > 0 {
			rpc = svc.RPCs[0]
		}
	}

	if rpc == nil {
		t.Fatalf("fixture d.zen has no rpc")
	}

	if ref, ok := permissionResourceRef(rpc); !ok || ref.Name != "User" {
		t.Errorf("permissionResourceRef() = %+v,%v, want User", ref, ok)
	}

	if ref, ok := ownerFieldRef(rpc); !ok || ref.Name != "owner_id" {
		t.Errorf("ownerFieldRef() = %+v,%v, want owner_id", ref, ok)
	}
}

// TestEntityRefsIn_skipsNonEntitiesAndEmptyNames pins the walk's guards: a
// service-only file yields no entity refs, and empty names contribute
// nothing.
func TestEntityRefsIn_skipsNonEntitiesAndEmptyNames(t *testing.T) {
	t.Parallel()

	if refs := entityRefsIn(nil); len(refs) != 0 {
		t.Errorf("entityRefsIn(nil) = %+v, want empty", refs)
	}

	parsed := parseRenameFiles(t, map[string]string{"c.zen": renameServiceSrc})
	got := 0

	for _, ref := range entityRefsIn(parsed["c.zen"]) {
		if ref.Name == "" {
			t.Errorf("entityRefsIn() yielded an empty name: %+v", ref)
		}

		got++
	}

	if got != 1 {
		t.Errorf("entityRefsIn(service fixture) = %d refs, want 1 (the rpc return type)", got)
	}

	empty := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{},
		&ast.EntityDecl{Name: "E", NamePos: diag.Position{File: "x.zen", Line: 1, Col: 1}, Relations: []*ast.RelationDecl{nil, {}}},
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{nil}},
	}}

	for _, ref := range entityRefsIn(empty) {
		if ref.Name == "" {
			t.Errorf("entityRefsIn() yielded an empty name: %+v", ref)
		}
	}
}

// TestFieldRefsIn_guards_pinSkips covers the per-entity scoping guards: a
// wrong entity name yields nothing, nil fields/indexes/rpcs are skipped,
// and a permission whose resource names another entity contributes no
// owner_field.
func TestFieldRefsIn_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if refs := fieldRefsIn(nil, "User"); len(refs) != 0 {
		t.Errorf("fieldRefsIn(nil) = %+v, want empty", refs)
	}

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameFieldUserSrc,
		"c.zen": renameServiceSrc,
		"d.zen": renamePermissionSrc,
		"e.zen": renameFieldOrderSrc,
	})

	if refs := fieldRefsIn(parsed["a.zen"], "Nobody"); len(refs) != 0 {
		t.Errorf("fieldRefsIn(unknown entity) = %+v, want empty", refs)
	}

	// c.zen's rpc carries no permission option at all, so its resource
	// lookup fails; d.zen's resource names User, not Task -- both hit the
	// skip guard for a non-matching permission resource.
	if refs := fieldRefsIn(parsed["c.zen"], "User"); len(refs) != 0 {
		t.Errorf("fieldRefsIn(rpc without permission) = %+v, want empty", refs)
	}

	if refs := fieldRefsIn(parsed["d.zen"], "Task"); len(refs) != 0 {
		t.Errorf("fieldRefsIn(other entity's resource) = %+v, want empty", refs)
	}

	// Short ColumnPos: the column still yields a ref with a zero position,
	// which renameSymbolEdits later filters via the Line <= 0 guard.
	short := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{
			Name:   "User",
			Fields: []*ast.FieldDecl{nil, {}},
			Indexes: []*ast.IndexDecl{nil, {
				Columns:   []string{"email"},
				ColumnPos: nil,
			}},
		},
		&ast.ServiceDecl{RPCs: []*ast.RPCDecl{nil}},
	}}

	if refs := fieldRefsIn(short, "User"); len(refs) != 1 {
		t.Errorf("fieldRefsIn(short positions) = %+v, want 1 column ref", refs)
	}
}

// TestJobRefsIn_guards_pinSkips pins the declaration and dispatch guards.
func TestJobRefsIn_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if refs := jobRefsIn(nil); len(refs) != 0 {
		t.Errorf("jobRefsIn(nil) = %+v, want empty", refs)
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.JobDecl{},
		&ast.ScheduleDecl{Dispatch: &ast.IdentValue{Name: "Nope"}},
		&ast.ScheduleDecl{Dispatch: &ast.CallValue{}},
		&ast.EntityDecl{Name: "User"},
	}}

	if refs := jobRefsIn(file); len(refs) != 0 {
		t.Errorf("jobRefsIn(guards) = %+v, want empty", refs)
	}
}

// TestServiceRefsIn_guards_pinSkips pins the declaration-only walk.
func TestServiceRefsIn_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if refs := serviceRefsIn(nil); len(refs) != 0 {
		t.Errorf("serviceRefsIn(nil) = %+v, want empty", refs)
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.ServiceDecl{},
		&ast.EntityDecl{Name: "User"},
	}}

	if refs := serviceRefsIn(file); len(refs) != 0 {
		t.Errorf("serviceRefsIn(empty) = %+v, want empty", refs)
	}
}

// TestRPCRefsIn_guards_pinSkips pins the per-service scoping guards.
func TestRPCRefsIn_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if refs := rpcRefsIn(nil, "S"); len(refs) != 0 {
		t.Errorf("rpcRefsIn(nil) = %+v, want empty", refs)
	}

	parsed := parseRenameFiles(t, map[string]string{"c.zen": renameRPCAdminSrc})

	if refs := rpcRefsIn(parsed["c.zen"], "Nobody"); len(refs) != 0 {
		t.Errorf("rpcRefsIn(unknown service) = %+v, want empty", refs)
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{nil, {}}},
		&ast.EntityDecl{Name: "User"},
	}}

	if refs := rpcRefsIn(file, "S"); len(refs) != 0 {
		t.Errorf("rpcRefsIn(guards) = %+v, want empty", refs)
	}
}

// TestEntityReferenceAt_miss_returnsFalse proves positions outside
// rpc-return/permission-resource references resolve to nothing.
func TestEntityReferenceAt_miss_returnsFalse(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameUserSrc})

	if ref, ok := entityReferenceAt(parsed["a.zen"], protocol.Position{Line: 99, Character: 0}); ok {
		t.Errorf("entityReferenceAt() = %+v,true, want not found", ref)
	}
}

// TestIndexColumnAt_guards_pinSkips covers the index walk's nil and miss
// paths, including a column whose position list is short.
func TestIndexColumnAt_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if _, _, ok := indexColumnAt(nil, protocol.Position{}); ok {
		t.Errorf("indexColumnAt(nil) = true, want false")
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.ServiceDecl{Name: "S"},
		&ast.EntityDecl{Name: "E", Indexes: []*ast.IndexDecl{nil}},
	}}

	if _, _, ok := indexColumnAt(file, protocol.Position{}); ok {
		t.Errorf("indexColumnAt() = true, want false")
	}

	parsed := parseRenameFiles(t, map[string]string{"a.zen": renameFieldUserSrc})

	entity, ref, ok := indexColumnAt(parsed["a.zen"], cursorOn(t, renameFieldUserSrc, 3, "email"))
	if !ok || ref.Name != "email" || entity == nil || entity.Name != "User" {
		t.Errorf("indexColumnAt() = %+v,%+v,%v, want User.email", entity, ref, ok)
	}

	if _, _, ok := indexColumnAt(parsed["a.zen"], protocol.Position{Line: 99, Character: 0}); ok {
		t.Errorf("indexColumnAt() past EOF = true, want false")
	}
}

// TestOwnerFieldAt_guards_pinSkips covers the owner_field walk's nil, miss,
// and resource-less paths.
func TestOwnerFieldAt_guards_pinSkips(t *testing.T) {
	t.Parallel()

	if _, _, ok := ownerFieldAt(nil, protocol.Position{}); ok {
		t.Errorf("ownerFieldAt(nil) = true, want false")
	}

	file := &ast.File{Decls: []ast.Decl{
		&ast.EntityDecl{Name: "User"},
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{nil}},
	}}

	if _, _, ok := ownerFieldAt(file, protocol.Position{}); ok {
		t.Errorf("ownerFieldAt() = true, want false")
	}

	// A permission call with owner_field but no resource argument still
	// resolves, with an empty entity name (e.g. mid-edit).
	midEdit := &ast.File{Decls: []ast.Decl{
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
			Permission: &ast.CallValue{Name: "check", Args: []*ast.Arg{
				{Pos: diag.Position{File: "m.zen", Line: 1, Col: 1}, Name: "owner_field",
					Value: &ast.IdentValue{Name: "email", Pos: diag.Position{File: "m.zen", Line: 1, Col: 1}}},
			}},
		}}},
	}}

	entityName, ref, ok := ownerFieldAt(midEdit, protocol.Position{Line: 0, Character: 0})
	if !ok || ref.Name != "email" || entityName != "" {
		t.Errorf("ownerFieldAt() = %q,%+v,%v, want empty entity with email", entityName, ref, ok)
	}

	parsed := parseRenameFiles(t, map[string]string{"d.zen": renameFieldOwnerSrc})

	// cursorOn lands on the label; the identifier itself starts past it.
	cursor := cursorOn(t, renameFieldOwnerSrc, 2, "owner_field: email")
	cursor.Character += uint32(len("owner_field: ")) //nolint:gosec // test fixture

	entityName, ref, ok = ownerFieldAt(parsed["d.zen"], cursor)
	if !ok || ref.Name != "email" || entityName != "User" {
		t.Errorf("ownerFieldAt() = %q,%+v,%v, want User.email", entityName, ref, ok)
	}
}

// TestRenameSymbolEdits_skipsBadRefsAndBrokenFiles proves the shared builder
// filters non-matching names and zero positions, skips nil siblings, and
// omits files left with no edits.
func TestRenameSymbolEdits_skipsBadRefsAndBrokenFiles(t *testing.T) {
	t.Parallel()

	parsed := parseRenameFiles(t, map[string]string{
		"a.zen": renameUserSrc,
		"e.zen": renameFieldOrderSrc,
	})
	parsed["broken.zen"] = nil

	// "Order" appears only in e.zen; a.zen's refs all mismatch and it must
	// be omitted from the result entirely.
	edit, err := renameEntityEdits(parsed, "Order", "Purchase")
	if err != nil {
		t.Fatalf("renameEntityEdits() error = %v, want nil", err)
	}

	if _, ok := edit.Changes[pathToURI("a.zen")]; ok {
		t.Errorf("a.zen present in Changes, want it omitted (no matches)")
	}

	if got := len(renameEditsFor(t, edit, pathToURI("e.zen"))); got != 1 {
		t.Errorf("e.zen edits = %d, want 1", got)
	}

	// A refsFn yielding only zero-position refs produces an (empty) edit
	// with no file entries rather than an error.
	zeroPos := map[string]*ast.File{"a.zen": parsed["a.zen"]}

	edit, err = renameSymbolEdits(zeroPos, "User", "Person", func(*ast.File) []symbolRef {
		return []symbolRef{{Name: "User"}}
	})
	if err != nil {
		t.Fatalf("renameSymbolEdits() error = %v, want nil", err)
	}

	if len(edit.Changes) != 0 {
		t.Errorf("Changes = %+v, want empty (zero positions filtered)", edit.Changes)
	}

	// An invalid new name is rejected even when files would match.
	if _, err := renameSymbolEdits(parsed, "User", "9bad", entityRefsIn); !errors.Is(err, errRenameInvalidIdent) {
		t.Errorf("renameSymbolEdits() error = %v, want errRenameInvalidIdent", err)
	}
}

// TestSortTextEdits_ordersByLineThenCharacter pins the deterministic
// ordering the builders promise regardless of walk order.
func TestSortTextEdits_ordersByLineThenCharacter(t *testing.T) {
	t.Parallel()

	edits := []protocol.TextEdit{
		{Range: protocol.Range{Start: protocol.Position{Line: 3, Character: 9}}},
		{Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 5}}},
		{Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 2}}},
	}

	sortTextEdits(edits)

	if edits[0].Range.Start.Line != 1 || edits[0].Range.Start.Character != 2 {
		t.Errorf("edits[0] = %+v, want line 1 char 2", edits[0].Range.Start)
	}

	if edits[1].Range.Start.Line != 1 || edits[1].Range.Start.Character != 5 {
		t.Errorf("edits[1] = %+v, want line 1 char 5", edits[1].Range.Start)
	}

	if edits[2].Range.Start.Line != 3 {
		t.Errorf("edits[2] = %+v, want line 3", edits[2].Range.Start)
	}
}
