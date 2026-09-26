package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.lsp.dev/uri"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/parser"
)

// zenExt is the file extension of zen DSL schema files.
const zenExt = ".zen"

// compileFunc matches compile.Compile's signature (with zero backends) so
// tests can substitute a spy and count how often a real compile happened.
type compileFunc func(files map[string]string) (*compile.Result, diag.List)

// Workspace holds every zen source the server knows about: live editor
// buffers (docs) layered over what was found on disk (diskCache). It caches
// the last compile keyed by a hash of the merged contents so that repeated
// requests between edits never re-run the compiler.
type Workspace struct {
	mu sync.Mutex

	rootPath  string
	docs      map[string]string
	diskCache map[string]string

	compile      compileFunc
	compileCount int

	lastHash   string
	lastResult *compile.Result
	lastDiags  diag.List

	// parsedCache holds the last parsed *ast.File per path, each tagged with
	// the content hash it was parsed from. references and rename consult
	// this (via ParsedFiles) instead of re-parsing every file on every
	// request; a per-file hash means editing one file only invalidates that
	// file's entry, not the whole workspace's.
	parsedCache map[string]parsedEntry
	parseCount  int
}

// parsedEntry pairs a parsed file with the content hash it was parsed from,
// so ParsedFiles can tell a cache hit from a stale entry with one comparison.
type parsedEntry struct {
	hash string
	file *ast.File
}

// NewWorkspace returns an empty workspace backed by the real compiler.
func NewWorkspace() *Workspace {
	return &Workspace{
		docs:        make(map[string]string),
		diskCache:   make(map[string]string),
		parsedCache: make(map[string]parsedEntry),
		compile: func(files map[string]string) (*compile.Result, diag.List) {
			return compile.Compile(files)
		},
	}
}

// SetRoot records the workspace root and scans it for .zen files on disk.
func (w *Workspace) SetRoot(rootURI string) {
	root, ok := uriToPath(rootURI)
	if !ok {
		return
	}

	w.mu.Lock()
	w.rootPath = root
	w.mu.Unlock()

	w.Rescan()
}

// Rescan walks the workspace root and refreshes diskCache from disk.
func (w *Workspace) Rescan() {
	w.mu.Lock()
	root := w.rootPath
	w.mu.Unlock()

	if root == "" {
		return
	}

	found := make(map[string]string)

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree must not abort the scan
		}

		name := d.Name()

		if d.IsDir() {
			if path != root && skipDir(name) {
				return filepath.SkipDir
			}

			return nil
		}

		if filepath.Ext(name) != zenExt {
			return nil
		}

		content, readErr := os.ReadFile(path) //nolint:gosec // path comes from walking the client-supplied root
		if readErr != nil {
			return nil //nolint:nilerr // skip unreadable files: WalkDirFunc nil means keep walking
		}

		abs, _ := filepath.Abs(path) //nolint:errcheck // path is absolute: joined from the absolute root, so Abs cannot fail

		found[abs] = string(content)

		return nil
	})

	w.mu.Lock()
	w.diskCache = found
	w.mu.Unlock()
}

// skipDir reports whether a directory should be excluded from the scan.
func skipDir(name string) bool {
	if name == "node_modules" {
		return true
	}

	return strings.HasPrefix(name, ".")
}

// SetDoc records the live content of an open buffer.
func (w *Workspace) SetDoc(uri, content string) {
	path, ok := uriToPath(uri)
	if !ok {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.docs[path] = content
}

// CloseDoc drops a buffer from the open set; the on-disk copy (if any) takes
// over again on the next compile.
func (w *Workspace) CloseDoc(uri string) {
	path, ok := uriToPath(uri)
	if !ok {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.docs, path)
}

// DocContent returns the best-known content for a document URI: the open
// buffer if there is one, otherwise the cached on-disk copy.
func (w *Workspace) DocContent(uri string) (string, bool) {
	path, ok := uriToPath(uri)
	if !ok {
		return "", false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if content, ok := w.docs[path]; ok {
		return content, true
	}

	content, cached := w.diskCache[path]

	return content, cached
}

// CollectAllFiles returns every known zen file keyed by absolute path, with
// open buffers overlaid on top of the on-disk snapshot.
func (w *Workspace) CollectAllFiles() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.collectLocked()
}

// collectLocked merges diskCache and docs with open buffers winning. Callers must hold w.mu.
func (w *Workspace) collectLocked() map[string]string {
	files := make(map[string]string, len(w.diskCache)+len(w.docs))

	for path, content := range w.diskCache {
		files[path] = content
	}

	for path, content := range w.docs {
		files[path] = content
	}

	return files
}

// Recompile runs the lex->parse->resolve pipeline over every known file,
// reusing the previous result when nothing has changed since the last call.
func (w *Workspace) Recompile() (*compile.Result, diag.List) {
	w.mu.Lock()
	files := w.collectLocked()
	hash := hashFiles(files)

	if w.lastResult != nil && hash == w.lastHash {
		result, diags := w.lastResult, w.lastDiags
		w.mu.Unlock()

		return result, diags
	}

	fn := w.compile
	w.mu.Unlock()

	result, diags := fn(files)

	w.mu.Lock()
	w.compileCount++
	w.lastHash = hash
	w.lastResult = result
	w.lastDiags = diags
	w.mu.Unlock()

	return result, diags
}

// CompileCount reports how many real compiles have run; used by tests to
// prove the content-hash cache actually short-circuits redundant work.
func (w *Workspace) CompileCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.compileCount
}

// ParsedFiles returns every known zen file's parsed AST, keyed by absolute
// path. It mirrors Recompile's content-hash short-circuit, but applied per
// file rather than to the workspace as a whole: a file whose content hash
// matches its last-parsed hash reuses the cached *ast.File, so editing one
// file does not force every other file to be re-parsed too. references and
// rename consult this instead of each calling parser.New(...).ParseFile()
// per file, per request.
func (w *Workspace) ParsedFiles() map[string]*ast.File {
	w.mu.Lock()
	files := w.collectLocked()
	cache := w.parsedCache
	w.mu.Unlock()

	result := make(map[string]*ast.File, len(files))
	next := make(map[string]parsedEntry, len(files))
	misses := 0

	for path, content := range files {
		hash := hashContent(content)

		if entry, ok := cache[path]; ok && entry.hash == hash {
			result[path] = entry.file
			next[path] = entry

			continue
		}

		file, _ := parser.New(path, []byte(content)).ParseFile()
		result[path] = file
		next[path] = parsedEntry{hash: hash, file: file}
		misses++
	}

	w.mu.Lock()
	w.parsedCache = next
	w.parseCount += misses
	w.mu.Unlock()

	return result
}

// ParseCount reports how many individual files have actually been parsed
// (cache misses) across every ParsedFiles call so far; used by tests to
// prove the per-file AST cache actually short-circuits redundant re-parsing.
func (w *Workspace) ParseCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.parseCount
}

// hashFiles produces a stable digest of a path->content map.
func hashFiles(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	h := sha256.New()

	for _, path := range paths {
		_, _ = h.Write([]byte(path))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(files[path]))
		_, _ = h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

// hashContent produces a stable digest of one file's content, the same
// technique hashFiles uses across a whole workspace, applied to a single
// file so ParsedFiles can invalidate one entry without touching the rest.
func hashContent(content string) string {
	sum := sha256.Sum256([]byte(content))

	return hex.EncodeToString(sum[:])
}

// uriToPath converts a file:// URI to an absolute filesystem path, reporting
// whether the conversion succeeded. A plain path is accepted as-is so tests
// and non-conforming clients still work. Non-file URIs (untitled:, https:,
// ...) have no filesystem path: they are rejected with ok=false, never a
// guess and never a panic.
func uriToPath(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}

	if scheme, _, _ := strings.Cut(raw, ":"); !strings.Contains(scheme, "/") && scheme != raw {
		// Has a URI scheme: only file URIs map onto the filesystem.
		// strings.Cut never fails, so a bare path (no colon at all, or a
		// colon past the first slash) falls through to the Abs branch below.
		u, err := uri.Parse(raw)
		if err != nil || !u.IsFile() {
			return "", false
		}

		return u.FsPath(), true
	}

	abs, _ := filepath.Abs(raw) //nolint:errcheck // best-effort mapping for non-conforming clients; Abs only fails when getwd does

	return abs, true
}

// pathToURI converts a filesystem path to a file:// URI. An input that is
// already a file URI passes through unchanged.
func pathToURI(path string) uri.URI {
	if path == "" {
		return ""
	}

	if scheme, _, _ := strings.Cut(path, ":"); !strings.Contains(scheme, "/") && scheme != path {
		if u, err := uri.Parse(path); err == nil && u.IsFile() {
			return u
		}
	}

	abs, _ := filepath.Abs(path) //nolint:errcheck // best-effort mapping; Abs only fails when getwd does

	return uri.File(abs)
}
