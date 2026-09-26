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
