package main

import (
	"sort"
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/parser"
)

// Refs fixtures use refs* names (not the rename* names the rename agent's
// tests own) so both test files can coexist in this package once rename
// lands.
const (
	refsUserSrc = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"}\n"

	refsTaskSrc = "entity Task {\n" +
		"\tid: uuid @primary\n" +
		"\tuser_id: uuid\n" +
		"\tbelongs_to user: User @foreign_key(user_id)\n" +
		"}\n"

	refsServiceSrc = "service UserService {\n" +
		"\trpc GetUser(id: uuid) -> User {\n" +
		"\t\tauth: none\n" +
		"\t}\n" +
		"}\n"

	refsFieldUserSrc = "entity User {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"\tindex(email)\n" +
		"}\n"

	refsFieldOrderSrc = "entity Order {\n" +
		"\tid: uuid @primary\n" +
		"\temail: string\n" +
		"}\n"

	refsJobSrc = "job CleanupJob(id: uuid) {\n" +
		"\tqueue: \"default\"\n" +
		"}\n"

	refsScheduleSrc = "schedule Nightly {\n" +
		"\tcron: \"0 0 * * *\"\n" +
		"\tdispatch: CleanupJob()\n" +
		"}\n"
)

// parseRefsFiles parses every fixture source, keyed by file name, exactly
// the way Workspace.ParsedFiles feeds referencesAt: one *ast.File per path.
func parseRefsFiles(t *testing.T, files map[string]string) map[string]*ast.File {
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

// locationsForURI returns the locations from locs whose URI matches uri,
// sorted by start position, so assertions don't depend on referencesAt's
// overall cross-file ordering.
func locationsForURI(locs protocol.LocationSlice, uri string) protocol.LocationSlice {
	var out protocol.LocationSlice

	for _, loc := range locs {
		if string(loc.URI) == uri {
			out = append(out, loc)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}

		return out[i].Range.Start.Character < out[j].Range.Start.Character
	})

	return out
}

// TestReferencesAtEntityReturnsEveryOccurrence proves entity references cover
// the declaration, the relation target, and the rpc return type across
// multiple files.
func TestReferencesAtEntityReturnsEveryOccurrence(t *testing.T) {
	files := map[string]string{
		"a.zen": refsUserSrc,
		"b.zen": refsTaskSrc,
		"c.zen": refsServiceSrc,
	}
	parsed := parseRefsFiles(t, files)

	locs := referencesAt(parsed["a.zen"], cursorOn(t, refsUserSrc, 0, "User"), parsed, true)

	if got := len(locationsForURI(locs, string(pathToURI("a.zen")))); got != 1 {
		t.Errorf("a.zen locations = %d, want 1 (the declaration)", got)
	}

	if got := len(locationsForURI(locs, string(pathToURI("b.zen")))); got != 1 {
		t.Errorf("b.zen locations = %d, want 1 (the relation target)", got)
	}

	if got := len(locationsForURI(locs, string(pathToURI("c.zen")))); got != 1 {
		t.Errorf("c.zen locations = %d, want 1 (the rpc return type)", got)
	}

	total := 0
	for _, name := range []string{"a.zen", "b.zen", "c.zen"} {
		total += len(locationsForURI(locs, string(pathToURI(name))))
	}

	if total != len(locs) {
		t.Errorf("locations total = %d, sum over files = %d, want equal (no stray files)", len(locs), total)
	}
}

// TestReferencesAtFieldIsEntityScoped proves field references return the
// declaration and the index column, while a same-named field on an
// unrelated entity is left out entirely.
func TestReferencesAtFieldIsEntityScoped(t *testing.T) {
	files := map[string]string{
		"a.zen": refsFieldUserSrc,
		"e.zen": refsFieldOrderSrc,
	}
	parsed := parseRefsFiles(t, files)

	locs := referencesAt(parsed["a.zen"], cursorOn(t, refsFieldUserSrc, 2, "email"), parsed, true)

	// a.zen: declaration (line 2) + index column (line 3).
	if got := len(locationsForURI(locs, string(pathToURI("a.zen")))); got != 2 {
		t.Errorf("a.zen locations = %d, want 2 (declaration + index column)", got)
	}

	if got := len(locationsForURI(locs, string(pathToURI("e.zen")))); got != 0 {
		t.Errorf("e.zen locations = %d, want 0 (Order.email is a different entity's field)", got)
	}
}

// TestReferencesAtJobReturnsDeclarationAndDispatches proves job references
// reach every dispatching schedule, across files.
func TestReferencesAtJobReturnsDeclarationAndDispatches(t *testing.T) {
	files := map[string]string{
		"a.zen": refsJobSrc,
		"b.zen": refsScheduleSrc,
	}
	parsed := parseRefsFiles(t, files)

	locs := referencesAt(parsed["a.zen"], cursorOn(t, refsJobSrc, 0, "CleanupJob"), parsed, true)

	if got := len(locationsForURI(locs, string(pathToURI("a.zen")))); got != 1 {
		t.Errorf("a.zen locations = %d, want 1 (the declaration)", got)
	}

	if got := len(locationsForURI(locs, string(pathToURI("b.zen")))); got != 1 {
		t.Errorf("b.zen locations = %d, want 1 (the dispatch)", got)
	}
}

// TestReferencesAtIncludeDeclarationToggles proves the includeDeclaration
// flag excludes/includes exactly the declaration's own position, leaving
// every other reference untouched either way.
func TestReferencesAtIncludeDeclarationToggles(t *testing.T) {
	files := map[string]string{
		"a.zen": refsUserSrc,
		"b.zen": refsTaskSrc,
	}
	parsed := parseRefsFiles(t, files)

	cursor := cursorOn(t, refsUserSrc, 0, "User")

	withDecl := referencesAt(parsed["a.zen"], cursor, parsed, true)
	withoutDecl := referencesAt(parsed["a.zen"], cursor, parsed, false)

	if len(withDecl) != len(withoutDecl)+1 {
		t.Fatalf("withDecl = %d, withoutDecl = %d, want withDecl exactly one more", len(withDecl), len(withoutDecl))
	}

	// The declaration itself must be gone from a.zen, but the relation
	// target in b.zen -- which is not the declaration -- must survive.
	if got := len(locationsForURI(withoutDecl, string(pathToURI("a.zen")))); got != 0 {
		t.Errorf("a.zen locations without declaration = %d, want 0", got)
	}

	if got := len(locationsForURI(withoutDecl, string(pathToURI("b.zen")))); got != 1 {
		t.Errorf("b.zen locations without declaration = %d, want 1 (unaffected)", got)
	}
}

// TestReferencesAtOnNonRenameableCursorReturnsNil proves the documented
// divergence from rename semantics: references on a cursor that resolves to
// nothing renameable is a normal empty result, never an error -- there is no
// error return at all in this signature, by design.
func TestReferencesAtOnNonRenameableCursorReturnsNil(t *testing.T) {
	files := map[string]string{
		"a.zen": refsUserSrc,
		"b.zen": refsTaskSrc,
	}
	parsed := parseRefsFiles(t, files)

	// `belongs_to user: User @foreign_key(user_id)` -- "user" here is the
	// relation's own field name, not a renameable symbol.
	if locs := referencesAt(parsed["b.zen"], cursorOn(t, refsTaskSrc, 3, "user:"), parsed, true); locs != nil {
		t.Errorf("referencesAt() = %+v, want nil", locs)
	}
}

// TestReferencesAtSkipsNilParsedFiles proves a hopelessly broken sibling
// file (a nil *ast.File in the workspace map) is skipped rather than
// dropping the whole request.
func TestReferencesAtSkipsNilParsedFiles(t *testing.T) {
	parsed := parseRefsFiles(t, map[string]string{
		"a.zen": refsUserSrc,
		"b.zen": refsTaskSrc,
	})
	parsed["broken.zen"] = nil

	locs := referencesAt(parsed["a.zen"], cursorOn(t, refsUserSrc, 0, "User"), parsed, true)

	if len(locs) != 2 {
		t.Errorf("referencesAt() = %d locations, want 2 (nil sibling skipped): %+v", len(locs), locs)
	}
}

// TestReferencesAtServiceAndRpc proves service- and rpc-kind cursors route
// to their own walks: the service declaration alone, and the RPC
// declaration scoped to its service.
func TestReferencesAtServiceAndRpc(t *testing.T) {
	parsed := parseRefsFiles(t, map[string]string{"c.zen": refsServiceSrc})

	svcLocs := referencesAt(parsed["c.zen"], cursorOn(t, refsServiceSrc, 0, "UserService"), parsed, true)
	if len(svcLocs) != 1 {
		t.Errorf("service references = %d, want 1 (the declaration): %+v", len(svcLocs), svcLocs)
	}

	rpcLocs := referencesAt(parsed["c.zen"], cursorOn(t, refsServiceSrc, 1, "GetUser"), parsed, true)
	if len(rpcLocs) != 1 {
		t.Errorf("rpc references = %d, want 1 (the declaration): %+v", len(rpcLocs), rpcLocs)
	}
}

// TestRefsFnForKind routes every known kind to its walk and rejects unknown
// kinds, pinning the full dispatch table including the default arm.
func TestRefsFnForKind(t *testing.T) {
	for _, kind := range []symbolKind{entityKind, fieldKind, jobKind, serviceKind, rpcKind} {
		t.Run(string(kind), func(t *testing.T) {
			target := renameTarget{Kind: kind, Name: "X", Owner: "Owner"}

			if fn := refsFnForKind(target); fn == nil {
				t.Errorf("refsFnForKind(%q) = nil, want a walk function", kind)
			}
		})
	}
}

// TestRefsFnForKindUnknownKindReturnsNil pins the default arm: a kind no
// workspace-wide walk knows about yields no walk function.
func TestRefsFnForKindUnknownKindReturnsNil(t *testing.T) {
	target := renameTarget{Kind: symbolKind("bogus"), Name: "X"}

	if fn := refsFnForKind(target); fn != nil {
		t.Errorf("refsFnForKind(bogus kind) = %T, want nil", fn)
	}
}

// TestSortLocationsOrdersByURIThenPosition proves the deterministic ordering
// referencesAt promises regardless of the files map's iteration order.
func TestSortLocationsOrdersByURIThenPosition(t *testing.T) {
	uriA := pathToURI("a.zen")
	uriB := pathToURI("b.zen")

	locs := protocol.LocationSlice{
		{URI: uriB, Range: protocol.Range{Start: protocol.Position{Line: 3}}},
		{URI: uriA, Range: protocol.Range{Start: protocol.Position{Line: 9}}},
		{URI: uriA, Range: protocol.Range{Start: protocol.Position{Line: 1}}},
		{URI: uriA, Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 5}}},
	}

	sortLocations(locs)

	if locs[0].URI != uriA || locs[0].Range.Start.Line != 1 || locs[0].Range.Start.Character != 0 {
		t.Errorf("locs[0] = %+v, want a.zen line 1 char 0", locs[0])
	}

	if locs[1].URI != uriA || locs[1].Range.Start.Line != 1 || locs[1].Range.Start.Character != 5 {
		t.Errorf("locs[1] = %+v, want a.zen line 1 char 5", locs[1])
	}

	if locs[2].URI != uriA || locs[2].Range.Start.Line != 9 {
		t.Errorf("locs[2] = %+v, want a.zen line 9", locs[2])
	}

	if locs[3].URI != uriB {
		t.Errorf("locs[3] = %+v, want b.zen", locs[3])
	}
}
