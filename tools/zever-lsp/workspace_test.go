package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
)

// writeFile creates a file under dir, making parent directories as needed.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	return path
}

// mustPath converts a URI test value to a path, failing on rejection.
func mustPath(t *testing.T, raw string) string {
	t.Helper()

	path, ok := uriToPath(raw)
	if !ok {
		t.Fatalf("uriToPath(%q) rejected a value the test expects to resolve", raw)
	}

	return path
}

// TestCollectAllFilesOverlaysBuffersOnDisk is the core layering rule: an open
// editor buffer must win over the stale bytes still sitting on disk.
func TestCollectAllFilesOverlaysBuffersOnDisk(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	onDisk := writeFile(t, dir, "a.zen", "entity FromDisk {\n\tid: uuid @primary\n}\n")
	writeFile(t, dir, "b.zen", "entity OnlyOnDisk {\n\tid: uuid @primary\n}\n")

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	files := ws.CollectAllFiles()
	if len(files) != 2 {
		t.Fatalf("scanned %d files, want 2: %v", len(files), keysOf(files))
	}

	// Now open a.zen with unsaved edits.
	buffer := "entity FromBuffer {\n\tid: uuid @primary\n}\n"
	ws.SetDoc(string(pathToURI(onDisk)), buffer)

	files = ws.CollectAllFiles()

	if len(files) != 2 {
		t.Fatalf("after opening a buffer got %d files, want 2: %v", len(files), keysOf(files))
	}

	abs, err := filepath.Abs(onDisk)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	if got := files[abs]; got != buffer {
		t.Errorf("open buffer did not win over disk:\ngot  %q\nwant %q", got, buffer)
	}

	// Closing the buffer restores the on-disk copy.
	ws.CloseDoc(string(pathToURI(onDisk)))

	files = ws.CollectAllFiles()
	if !strings.Contains(files[abs], "FromDisk") {
		t.Errorf("after closing the buffer, content = %q, want the on-disk copy", files[abs])
	}
}

// TestCollectAllFilesIncludesUnsavedNewFile covers a buffer that has no
// on-disk counterpart yet.
func TestCollectAllFilesIncludesUnsavedNewFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	newPath := filepath.Join(dir, "fresh.zen")
	ws.SetDoc(string(pathToURI(newPath)), "entity Fresh {\n\tid: uuid @primary\n}\n")

	files := ws.CollectAllFiles()
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %v", len(files), keysOf(files))
	}
}

func TestScanSkipsHiddenAndVendorDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeFile(t, dir, "keep.zen", "entity Keep {\n\tid: uuid @primary\n}\n")
	writeFile(t, dir, filepath.Join(".git", "hidden.zen"), "entity Hidden {}\n")
	writeFile(t, dir, filepath.Join("node_modules", "dep.zen"), "entity Dep {}\n")
	writeFile(t, dir, filepath.Join(".hidden", "nested.zen"), "entity Nested {}\n")
	writeFile(t, dir, "notes.txt", "not a schema")

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	files := ws.CollectAllFiles()

	if len(files) != 1 {
		t.Fatalf("scanned %v, want only keep.zen", keysOf(files))
	}

	for path := range files {
		if filepath.Base(path) != "keep.zen" {
			t.Errorf("scanned %q, want keep.zen", path)
		}
	}
}

// TestRescanFindsNestedAndSkipsUnreadable covers the walk branches the skip
// test above does not: a .zen file inside a normal subdirectory is found,
// while an unreadable entry (here a dangling symlink) is skipped without
// aborting the scan.
func TestRescanFindsNestedAndSkipsUnreadable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeFile(t, dir, "keep.zen", "entity Keep {}\n")
	writeFile(t, dir, filepath.Join("pkg", "nested.zen"), "entity Nested {}\n")

	bad := filepath.Join(dir, "bad.zen")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist.zen"), bad); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	files := ws.CollectAllFiles()
	if len(files) != 2 {
		t.Fatalf("scanned %v, want keep.zen and pkg/nested.zen", keysOf(files))
	}
}

// TestRescanWithoutRootDoesNothing proves Rescan on a fresh workspace (and a
// SetRoot it cannot resolve) leaves the workspace empty instead of scanning
// the process working directory.
func TestRescanWithoutRootDoesNothing(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()
	ws.Rescan()

	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Fatalf("Rescan without root found %v, want nothing", keysOf(files))
	}

	ws.SetRoot("")
	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Fatalf("SetRoot(\"\") found %v, want nothing", keysOf(files))
	}

	ws.SetRoot("untitled:Untitled-1")
	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Fatalf("SetRoot(non-file URI) found %v, want nothing", keysOf(files))
	}

	ws.SetRoot("https://example.com/workspace")
	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Fatalf("SetRoot(https URI) found %v, want nothing", keysOf(files))
	}
}

// TestRescanMissingRootStaysEmpty covers the walk-error branch: a root that
// resolves but does not exist reports nothing instead of failing.
func TestRescanMissingRootStaysEmpty(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "nope")

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(missing)))

	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Fatalf("missing root found %v, want nothing", keysOf(files))
	}
}

// TestNonFileURIsAreIgnored proves buffers and lookups addressed by
// non-file URIs never enter the workspace maps.
func TestNonFileURIsAreIgnored(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()
	ws.SetDoc("untitled:Untitled-1", "entity Ghost {}\n")
	ws.CloseDoc("untitled:Untitled-1")

	if _, ok := ws.DocContent("untitled:Untitled-1"); ok {
		t.Error("DocContent reported a non-file URI as known")
	}

	if files := ws.CollectAllFiles(); len(files) != 0 {
		t.Errorf("non-file buffer leaked into files: %v", keysOf(files))
	}
}

// TestRecompileCachesUnchangedContent proves the content-hash cache actually
// short-circuits the compiler rather than merely returning equal output.
func TestRecompileCachesUnchangedContent(t *testing.T) {
	t.Parallel()

	calls := 0

	ws := NewWorkspace()
	ws.compile = func(files map[string]string) (*compile.Result, diag.List) {
		calls++

		return compile.Compile(files)
	}

	uri := string(pathToURI(filepath.Join(t.TempDir(), "a.zen")))
	ws.SetDoc(uri, "entity User {\n\tid: uuid @primary\n}\n")

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if calls != 1 {
		t.Fatalf("after the first Recompile the spy saw %d calls, want 1", calls)
	}

	// Nothing changed: three more requests must all be served from cache.
	for i := range 3 {
		if _, diags := ws.Recompile(); diags.HasErrors() {
			t.Fatalf("cached recompile %d returned diagnostics: %v", i, diags)
		}
	}

	if calls != 1 {
		t.Errorf("the spy saw %d compiles across 4 Recompile calls, want 1 (cache did not skip)", calls)
	}

	if got := ws.CompileCount(); got != 1 {
		t.Errorf("CompileCount() = %d, want 1", got)
	}

	// A real edit must invalidate the cache.
	ws.SetDoc(uri, "entity User {\n\tid: uuid @primary\n\temail: string\n}\n")

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after edit: %v", diags)
	}

	if calls != 2 {
		t.Errorf("after an edit the spy saw %d calls, want 2 (cache did not invalidate)", calls)
	}

	// Reverting to the original text is a content change too, and the hash is
	// over content, so this legitimately hits the compiler again.
	ws.CloseDoc(uri)

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after close: %v", diags)
	}

	if calls != 3 {
		t.Errorf("after closing the only document the spy saw %d calls, want 3", calls)
	}
}

// TestParsedFilesCachesUnchangedContent proves ParsedFiles' per-file
// content-hash cache actually short-circuits parsing rather than merely
// returning equal ASTs -- the same proof shape as
// TestRecompileCachesUnchangedContent, but for the per-file AST cache that
// references and rename consume.
func TestParsedFilesCachesUnchangedContent(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()

	uriA := string(pathToURI(filepath.Join(t.TempDir(), "a.zen")))
	uriB := string(pathToURI(filepath.Join(t.TempDir(), "b.zen")))
	ws.SetDoc(uriA, "entity User {\n\tid: uuid @primary\n}\n")
	ws.SetDoc(uriB, "entity Task {\n\tid: uuid @primary\n}\n")

	files := ws.ParsedFiles()
	if len(files) != 2 {
		t.Fatalf("ParsedFiles() = %d files, want 2", len(files))
	}

	if got := ws.ParseCount(); got != 2 {
		t.Fatalf("after the first ParsedFiles the parse count = %d, want 2 (one per file)", got)
	}

	// Nothing changed: three more calls must all be served from cache.
	for i := range 3 {
		if _, ok := ws.ParsedFiles()[mustPath(t, uriA)]; !ok {
			t.Fatalf("ParsedFiles call %d lost a.zen", i)
		}
	}

	if got := ws.ParseCount(); got != 2 {
		t.Errorf("parse count after 4 ParsedFiles calls = %d, want 2 (cache did not skip)", got)
	}

	// Editing only a.zen must re-parse a.zen but leave b.zen's cache entry
	// alone -- proving the cache is keyed per file, not per workspace.
	ws.SetDoc(uriA, "entity User {\n\tid: uuid @primary\n\temail: string\n}\n")

	files = ws.ParsedFiles()
	if got := ws.ParseCount(); got != 3 {
		t.Errorf("parse count after editing one file = %d, want 3 (only that file re-parsed)", got)
	}

	if got := len(files[mustPath(t, uriA)].Decls); got == 0 {
		t.Errorf("a.zen's cached AST has no decls after re-parse")
	}
}

// TestParsedFilesInvalidatesEditedFile proves an edit to a file's content is
// visible in the very next ParsedFiles call -- the cache must never serve a
// stale AST after didChange updates a buffer.
func TestParsedFilesInvalidatesEditedFile(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()

	uri := string(pathToURI(filepath.Join(t.TempDir(), "a.zen")))
	ws.SetDoc(uri, "entity User {\n\tid: uuid @primary\n}\n")

	path := mustPath(t, uri)

	before := ws.ParsedFiles()[path]
	if before == nil || len(before.Decls) != 1 {
		t.Fatalf("initial parse: got %+v, want exactly one decl", before)
	}

	ws.SetDoc(uri, "entity User {\n\tid: uuid @primary\n}\nentity Task {\n\tid: uuid @primary\n}\n")

	after := ws.ParsedFiles()[path]
	if after == nil || len(after.Decls) != 2 {
		t.Fatalf("after edit: got %+v, want exactly two decls (the update must not be served stale)", after)
	}
}

// TestParsedFilesToleratesUnparseableContent proves a buffer that does not
// parse still occupies its cache slot without breaking its neighbours.
func TestParsedFilesToleratesUnparseableContent(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()

	badURI := string(pathToURI(filepath.Join(t.TempDir(), "bad.zen")))
	goodURI := string(pathToURI(filepath.Join(t.TempDir(), "good.zen")))
	ws.SetDoc(badURI, "entity {\n\tthis is not valid\n")
	ws.SetDoc(goodURI, "entity Good {\n\tid: uuid @primary\n}\n")

	files := ws.ParsedFiles()
	if len(files) != 2 {
		t.Fatalf("ParsedFiles() = %d files, want 2", len(files))
	}

	if got := ws.ParseCount(); got != 2 {
		t.Errorf("ParseCount() = %d, want 2", got)
	}

	if got := files[mustPath(t, goodURI)]; got == nil || len(got.Decls) != 1 {
		t.Errorf("valid neighbour parsed as %+v, want one decl", got)
	}
}

// TestWorkspaceConcurrentAccessSafe hammers the shared maps from several
// goroutines; the race detector turns any missing lock into a failure.
func TestWorkspaceConcurrentAccessSafe(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace()

	uri := string(pathToURI(filepath.Join(t.TempDir(), "a.zen")))

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ws.SetDoc(uri, "entity User {}\n")
			_, _ = ws.DocContent(uri)
			_ = ws.CollectAllFiles()
			_, _ = ws.Recompile()
			_ = ws.ParsedFiles()
			_ = ws.CompileCount()
			_ = ws.ParseCount()
			_ = i
		}()
	}

	wg.Wait()
}

func TestHashFilesIsOrderIndependent(t *testing.T) {
	t.Parallel()

	a := map[string]string{"x.zen": "one", "y.zen": "two"}
	b := map[string]string{"y.zen": "two", "x.zen": "one"}

	if hashFiles(a) != hashFiles(b) {
		t.Error("hashFiles depends on map iteration order")
	}

	c := map[string]string{"x.zen": "one", "y.zen": "three"}
	if hashFiles(a) == hashFiles(c) {
		t.Error("hashFiles collided on different content")
	}

	// Path and content are separated, so moving a boundary changes the hash.
	d := map[string]string{"x.zeny.zen": "onetwo"}
	if hashFiles(a) == hashFiles(d) {
		t.Error("hashFiles collided across a path/content boundary")
	}

	if hashContent("one") == hashContent("two") {
		t.Error("hashContent collided on different content")
	}
}

func TestURIRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a b.zen")

	uri := pathToURI(path)

	if got := mustPath(t, string(uri)); got != path {
		t.Errorf("round trip: uriToPath(%q) = %q, want %q", uri, got, path)
	}

	// A bare path is tolerated for clients and tests that pass one.
	if got := mustPath(t, path); got != path {
		t.Errorf("uriToPath on a bare path = %q, want %q", got, path)
	}

	if _, ok := uriToPath(""); ok {
		t.Error("uriToPath(\"\") reported ok, want rejection")
	}
}

// TestURIToPathDecisions tables the accept/reject rules: file URIs resolve,
// bare paths are tolerated, and anything with a non-file scheme is rejected
// without a guess.
func TestURIToPathDecisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantOK  bool
		absOfIn bool
	}{
		{name: "empty is rejected", in: "", wantOK: false},
		{name: "untitled is rejected", in: "untitled:Untitled-1", wantOK: false},
		{name: "https is rejected", in: "https://example.com/w/a.zen", wantOK: false},
		{name: "custom scheme is rejected", in: "vscode-remote://host/path/a.zen", wantOK: false},
		{name: "colon without slash is a scheme and rejected", in: "a:b", wantOK: false},
		{name: "absolute bare path is tolerated", in: filepath.Join(t.TempDir(), "a.zen"), wantOK: true},
		{name: "relative bare path resolves against cwd", in: "rel-path-doc.zen", wantOK: true, absOfIn: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := uriToPath(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("uriToPath(%q) ok = %v, want %v", tc.in, ok, tc.wantOK)
			}

			if !tc.wantOK {
				return
			}

			want := tc.in
			if tc.absOfIn {
				abs, err := filepath.Abs(tc.in)
				if err != nil {
					t.Fatalf("Abs: %v", err)
				}

				want = abs
			}

			if got != want {
				t.Errorf("uriToPath(%q) = %q, want %q", tc.in, got, want)
			}
		})
	}
}

// TestPathToURIDecisions tables the reverse mapping: empty stays empty, an
// existing file URI passes through, and plain paths become file URIs.
func TestPathToURIDecisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "a b.zen")

	// Empty maps to the zero URI.
	if got := pathToURI(""); got != "" {
		t.Errorf("pathToURI(\"\") = %q, want \"\"", got)
	}

	// A plain absolute path becomes a file URI that resolves back.
	first := pathToURI(path)
	if got := mustPath(t, string(first)); got != path {
		t.Errorf("round trip through %q gave %q, want %q", first, got, path)
	}

	// An existing file URI passes through unchanged (canonical form).
	if again := pathToURI(string(first)); again != first {
		t.Errorf("pathToURI(%q) = %q, want passthrough %q", first, again, first)
	}

	// A relative path resolves against the working directory.
	rel := string(pathToURI("fresh.zen"))
	abs, err := filepath.Abs("fresh.zen")
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	if got := mustPath(t, rel); got != abs {
		t.Errorf("pathToURI(relative) resolved to %q, want %q", got, abs)
	}
}

func TestDocContentPrefersBuffer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := writeFile(t, dir, "a.zen", "on disk")

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	got, ok := ws.DocContent(string(pathToURI(path)))
	if !ok || got != "on disk" {
		t.Fatalf("DocContent = %q, %v; want the on-disk copy", got, ok)
	}

	ws.SetDoc(string(pathToURI(path)), "in buffer")

	got, ok = ws.DocContent(string(pathToURI(path)))
	if !ok || got != "in buffer" {
		t.Errorf("DocContent = %q, %v; want the buffer copy", got, ok)
	}

	if _, ok := ws.DocContent(string(pathToURI(filepath.Join(dir, "missing.zen")))); ok {
		t.Error("DocContent reported an unknown file as known")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

// TestAddDiskFileDecisions tables the surgical insert rules: a .zen file on
// disk joins the snapshot with its disk content, while a non-.zen path, an
// unreadable path, and a non-file URI are ignored without touching the rest
// of the snapshot.
func TestAddDiskFileDecisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	const zenContent = "entity Fresh {\n\tid: uuid @primary\n}\n"

	zenPath := writeFile(t, dir, "fresh.zen", zenContent)
	txtPath := writeFile(t, dir, "notes.txt", "not a schema")

	tests := []struct {
		name      string
		path      string
		wantKnown bool
	}{
		{"zen file from disk is added", zenPath, true},
		{"non-zen file is ignored", txtPath, false},
		{"unreadable path is ignored", filepath.Join(dir, "missing.zen"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ws := NewWorkspace()
			ws.AddDiskFile(tc.path)

			abs, err := filepath.Abs(tc.path)
			if err != nil {
				t.Fatalf("Abs: %v", err)
			}

			files := ws.CollectAllFiles()

			got, ok := files[abs]
			if ok != tc.wantKnown {
				t.Fatalf("CollectAllFiles knows %q = %v, want %v (all: %v)", abs, ok, tc.wantKnown, keysOf(files))
			}

			if tc.wantKnown && got != zenContent {
				t.Errorf("added content = %q, want the disk bytes %q", got, zenContent)
			}
		})
	}
}

// TestRemoveDiskFileDecisions proves a delete drops exactly its path: the
// workspace forgets it, an unknown path is a no-op, and an open buffer for
// the same path keeps winning in CollectAllFiles until it is closed.
func TestRemoveDiskFileDecisions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	seed := writeFile(t, dir, "seed.zen", "entity Seed {\n\tid: uuid @primary\n}\n")
	gone := writeFile(t, dir, "gone.zen", "entity Gone {\n\tid: uuid @primary\n}\n")

	seedAbs, err := filepath.Abs(seed)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	goneAbs, err := filepath.Abs(gone)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(dir)))

	if files := ws.CollectAllFiles(); len(files) != 2 {
		t.Fatalf("seeded %d files, want 2: %v", len(files), keysOf(files))
	}

	ws.RemoveDiskFile(goneAbs)

	if files := ws.CollectAllFiles(); len(files) != 1 {
		t.Fatalf("after removing one file got %d, want 1: %v", len(files), keysOf(files))
	} else if _, ok := files[seedAbs]; !ok {
		t.Errorf("after removing gone.zen the survivor is %v, want seed.zen kept", keysOf(files))
	}

	// Removing again, or a path never known, changes nothing.
	ws.RemoveDiskFile(goneAbs)
	ws.RemoveDiskFile(filepath.Join(dir, "missing.zen"))

	if files := ws.CollectAllFiles(); len(files) != 1 {
		t.Errorf("after no-op removes got %d files, want 1: %v", len(files), keysOf(files))
	}

	// An open buffer survives the on-disk delete: the buffer still wins the
	// layering until DidClose drops it.
	ws.SetDoc(string(pathToURI(seed)), "entity SeedEdited {\n\tid: uuid @primary\n}\n")
	ws.RemoveDiskFile(seedAbs)

	if got, ok := ws.CollectAllFiles()[seedAbs]; !ok || !strings.Contains(got, "SeedEdited") {
		t.Errorf("buffer after disk delete = (%q, %v), want the open buffer kept", got, ok)
	}
}

// multiRootEntityDoc is a schema that compiles cleanly on its own; declaring
// it in two roots at once is only an error when the roots are merged into
// one compile.
const multiRootEntityDoc = "entity User {\n\tid: uuid @primary\n}\n"

// TestRecompileIsolatesRootsNoDuplicateDiagnostic is THE isolation proof:
// the same entity declared in two independent roots must not collide, while
// merging the same files into one compile still reports the duplicate (so
// the test cannot pass vacuously if the resolver ever stops flagging it).
func TestRecompileIsolatesRootsNoDuplicateDiagnostic(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()

	writeFile(t, rootA, filepath.Join("schema", "user.zen"), multiRootEntityDoc)
	writeFile(t, rootB, filepath.Join("schema", "user.zen"), multiRootEntityDoc)

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(rootA)))
	ws.AddRoot(string(pathToURI(rootB)))

	if _, merged := compile.Compile(ws.CollectAllFiles()); !merged.HasErrors() {
		t.Fatal("merged compile of the two roots is clean; the test cannot prove isolation")
	}

	result, diags := ws.Recompile()
	if diags.HasErrors() {
		t.Fatalf("per-root Recompile reported diagnostics: %v", diags)
	}

	if result == nil || result.Schema == nil {
		t.Fatal("per-root Recompile returned no primary schema")
	}
}

// TestRecompileAllMergesDiagnosticsAcrossRoots proves the merged list carries
// every root's diagnostics while the primary result stays usable: root B is
// broken, so the merged list must err, root A's entry must stay clean, and
// the primary (root A) schema must still resolve.
func TestRecompileAllMergesDiagnosticsAcrossRoots(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()

	writeFile(t, rootA, filepath.Join("schema", "a.zen"), multiRootEntityDoc)
	writeFile(t, rootB, filepath.Join("schema", "b.zen"), "entity User {\n\tid: strnig\n}\n")

	ws := NewWorkspace()
	ws.SetRoot(string(pathToURI(rootA)))
	ws.AddRoot(string(pathToURI(rootB)))

	absA := mustPath(t, string(pathToURI(rootA)))
	absB := mustPath(t, string(pathToURI(rootB)))

	primary, all, perRoot := ws.RecompileAll()
	if !all.HasErrors() {
		t.Fatal("RecompileAll merged diagnostics have no errors, want root B's failure")
	}

	if len(perRoot) != 2 {
		t.Fatalf("perRoot has %d entries, want 2", len(perRoot))
	}

	if got := perRoot[absA]; got.HasErrors() {
		t.Errorf("primary root diagnostics = %v, want clean", got)
	}

	if got := perRoot[absB]; !got.HasErrors() {
		t.Error("non-primary root diagnostics are clean, want root B's failure")
	}

	if primary == nil || primary.Schema == nil {
		t.Error("primary result has no schema, want root A's schema despite root B failing")
	}

	// Recompile delegates: the primary result plus the same merged list.
	if _, diags := ws.Recompile(); len(diags) != len(all) {
		t.Errorf("Recompile merged %d diagnostics, want %d like RecompileAll", len(diags), len(all))
	}
}

// TestRecompilePerRootCacheSkipsCleanRoots proves the compile cache is keyed
// per root: editing a file in one root recompiles exactly that root.
func TestRecompilePerRootCacheSkipsCleanRoots(t *testing.T) {
	t.Parallel()

	rootA := t.TempDir()
	rootB := t.TempDir()

	fileA := writeFile(t, rootA, filepath.Join("schema", "a.zen"), "entity Alpha {\n\tid: uuid @primary\n}\n")
	fileB := writeFile(t, rootB, filepath.Join("schema", "b.zen"), "entity Beta {\n\tid: uuid @primary\n}\n")

	ws := NewWorkspace()

	var mu sync.Mutex
	calls := 0

	ws.compile = func(files map[string]string) (*compile.Result, diag.List) {
		mu.Lock()
		calls++
		mu.Unlock()

		return compile.Compile(files)
	}

	count := func() int {
		mu.Lock()
		defer mu.Unlock()

		return calls
	}

	ws.SetRoot(string(pathToURI(rootA)))
	ws.AddRoot(string(pathToURI(rootB)))

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := count(); got != 2 {
		t.Fatalf("first Recompile ran %d compiles, want 2 (one per root)", got)
	}

	if got := ws.CompileCount(); got != 2 {
		t.Fatalf("CompileCount() = %d, want 2", got)
	}

	// Nothing changed: the next call must be served from cache.
	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if got := count(); got != 2 {
		t.Errorf("clean Recompile ran %d compiles total, want 2 (cache did not skip)", got)
	}

	// Editing only root A's file must recompile exactly root A.
	ws.SetDoc(string(pathToURI(fileA)), "entity Alpha {\n\tid: uuid @primary\n\tname: string\n}\n")

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after editing root A: %v", diags)
	}

	if got := count(); got != 3 {
		t.Errorf("after editing root A ran %d compiles total, want 3 (only A again)", got)
	}

	// Editing only root B's file must recompile exactly root B.
	ws.SetDoc(string(pathToURI(fileB)), "entity Beta {\n\tid: uuid @primary\n\tname: string\n}\n")

	if _, diags := ws.Recompile(); diags.HasErrors() {
		t.Fatalf("unexpected diagnostics after editing root B: %v", diags)
	}

	if got := count(); got != 4 {
		t.Errorf("after editing root B ran %d compiles total, want 4 (only B again)", got)
	}

	if got := ws.CompileCount(); got != 4 {
		t.Errorf("CompileCount() = %d, want 4", got)
	}
}

// TestPartitionFilesByRootDecisions tables the ownership rules: the longest
// root prefix wins, a sibling sharing a string prefix does not match, files
// outside every root fall back to primary, and no roots means one group.
func TestPartitionFilesByRootDecisions(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	parent := filepath.Join(base, "proj")
	child := filepath.Join(parent, "sub")
	sibling := filepath.Join(base, "proj-sibling", "x.zen")
	inParent := filepath.Join(parent, "a.zen")
	inChild := filepath.Join(child, "b.zen")
	outside := filepath.Join(base, "elsewhere", "c.zen")

	tests := []struct {
		name    string
		roots   []string
		primary string
		files   map[string]string
		want    map[string][]string
	}{
		{
			name:    "single root owns everything under it",
			roots:   []string{parent},
			primary: parent,
			files:   map[string]string{inParent: "a", inChild: "b"},
			want:    map[string][]string{parent: {inParent, inChild}},
		},
		{
			name:    "nested root wins over its parent",
			roots:   []string{parent, child},
			primary: parent,
			files:   map[string]string{inParent: "a", inChild: "b"},
			want:    map[string][]string{parent: {inParent}, child: {inChild}},
		},
		{
			name:    "sibling sharing a string prefix does not match",
			roots:   []string{parent},
			primary: parent,
			files:   map[string]string{sibling: "x"},
			want:    map[string][]string{parent: {sibling}},
		},
		{
			name:    "outside file falls back to primary",
			roots:   []string{parent, child},
			primary: parent,
			files:   map[string]string{outside: "c"},
			want:    map[string][]string{parent: {outside}, child: {}},
		},
		{
			name:    "no roots groups everything under primary",
			roots:   nil,
			primary: "",
			files:   map[string]string{inParent: "a", outside: "c"},
			want:    map[string][]string{"": {inParent, outside}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := partitionFilesByRoot(tc.files, tc.roots, tc.primary)
			if len(got) != len(tc.want) {
				t.Fatalf("partitioned into %d groups, want %d: %v", len(got), len(tc.want), got)
			}

			for root, wantPaths := range tc.want {
				group, ok := got[root]
				if !ok {
					t.Fatalf("missing group for root %q (got %v)", root, got)
				}

				if len(group) != len(wantPaths) {
					t.Errorf("root %q owns %d files, want %d: %v", root, len(group), len(wantPaths), keysOf(group))
					continue
				}

				for _, path := range wantPaths {
					if _, ok := group[path]; !ok {
						t.Errorf("root %q missing file %q (owns %v)", root, path, keysOf(group))
					}
				}
			}
		})
	}
}
