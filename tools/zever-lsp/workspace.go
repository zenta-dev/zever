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

// schemaDirName is the schema subtree name handed to compile.WithSchemaDir:
// each workspace root owns its own schema/ subtree, so dir-derived module
// assignment stays correct per root while duplicate-declaration detection
// never leaks across roots.
const schemaDirName = "schema"

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
	roots     []string
	docs      map[string]string
	diskCache map[string]string

	compile      compileFunc
	compileCount int

	// cached holds the last compile per workspace root, each tagged with
	// the content hash of that root's file set. Editing a file in one
	// root invalidates only that root's entry, so the other roots are
	// never recompiled; a single-root workspace holds exactly one entry
	// and behaves exactly like the old whole-workspace hash cache.
	cached map[string]cachedEntry

	// parsedCache holds the last parsed *ast.File per path, each tagged with
	// the content hash it was parsed from. references and rename consult
	// this (via ParsedFiles) instead of re-parsing every file on every
	// request; a per-file hash means editing one file only invalidates that
	// file's entry, not the whole workspace's.
	parsedCache map[string]parsedEntry
	parseCount  int
}

// cachedEntry pairs a root's last compile output with the content hash it
// was compiled from, so RecompileAll can tell a cache hit from a stale
// entry with one comparison.
type cachedEntry struct {
	hash   string
	result *compile.Result
	diags  diag.List
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
		cached:      make(map[string]cachedEntry),
		parsedCache: make(map[string]parsedEntry),
		compile: func(files map[string]string) (*compile.Result, diag.List) {
			return compile.WithSchemaDir(files, schemaDirName)
		},
	}
}

// SetRoot replaces the workspace roots with a single root and scans it.
func (w *Workspace) SetRoot(rootURI string) {
	root, ok := uriToPath(rootURI)
	if !ok {
		return
	}

	w.mu.Lock()
	w.rootPath = filepath.Clean(root)
	w.roots = []string{filepath.Clean(root)}
	w.mu.Unlock()

	w.Rescan()
}

// AddRoot adds a workspace root (if not already present) and rescans.
func (w *Workspace) AddRoot(rootURI string) {
	root, ok := uriToPath(rootURI)
	if !ok {
		return
	}

	w.mu.Lock()

	found := false
	for _, r := range w.roots {
		if r == filepath.Clean(root) {
			found = true
			break
		}
	}

	if !found {
		w.roots = append(w.roots, filepath.Clean(root))
	}

	if w.rootPath == "" && len(w.roots) > 0 {
		w.rootPath = w.roots[0]
	}

	w.mu.Unlock()

	w.Rescan()
}

// RemoveRoot drops a workspace root and rescans.
func (w *Workspace) RemoveRoot(rootURI string) {
	root, ok := uriToPath(rootURI)
	if !ok {
		return
	}

	w.mu.Lock()

	filtered := make([]string, 0, len(w.roots))
	for _, r := range w.roots {
		if r != filepath.Clean(root) {
			filtered = append(filtered, r)
		}
	}

	w.roots = filtered

	if len(w.roots) == 0 {
		w.rootPath = ""
	} else if w.rootPath == root {
		w.rootPath = w.roots[0]
	}

	w.mu.Unlock()

	w.Rescan()
}

// Rescan walks every workspace root and refreshes diskCache from disk.
func (w *Workspace) Rescan() {
	w.mu.Lock()
	roots := append([]string(nil), w.roots...)
	if len(roots) == 0 && w.rootPath != "" {
		roots = []string{w.rootPath}
	}
	w.mu.Unlock()

	if len(roots) == 0 {
		return
	}

	found := make(map[string]string)

	for _, root := range roots {
		if root == "" {
			continue
		}

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

			content, readErr := os.ReadFile(path) //nolint:gosec // G122: path comes from walking the user-configured project root; LSP is a dev tool, not a production service
			if readErr != nil {
				return nil //nolint:nilerr // skip unreadable files: WalkDirFunc nil means keep walking
			}

			abs, _ := filepath.Abs(path) // path is absolute: joined from the absolute root, so Abs cannot fail

			found[abs] = string(content)

			return nil
		})
	}

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

// AddDiskFile inserts one on-disk file into the snapshot without a full
// Rescan. The content is read from disk best-effort, exactly like Rescan
// reads each file it walks: a non-.zen path or an unreadable file is
// ignored. Callers pass the absolute path uriToPath resolves the file URI
// to; it is normalized the same way Rescan normalizes walked paths.
func (w *Workspace) AddDiskFile(path string) {
	if filepath.Ext(path) != zenExt {
		return
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return
	}

	abs, _ := filepath.Abs(path) // path is already absolute, so Abs cannot fail

	w.mu.Lock()
	defer w.mu.Unlock()

	w.diskCache[abs] = string(content)
}

// RemoveDiskFile drops one path from the on-disk snapshot without a full
// Rescan. Unknown paths are a no-op; an open buffer for the same path is
// untouched, so it keeps winning in CollectAllFiles until it is closed.
func (w *Workspace) RemoveDiskFile(path string) {
	abs, _ := filepath.Abs(path) // normalize like AddDiskFile so both spellings of a path meet

	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.diskCache, abs)
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

	return w.ContentForPath(path)
}

// ContentForPath returns the best-known content for an absolute path: the
// open buffer if there is one, otherwise the cached on-disk copy. It is the
// path-keyed counterpart of DocContent, used where a compiler diagnostic
// reports a path rather than a URI.
func (w *Workspace) ContentForPath(path string) (string, bool) {
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

// RecompileAll compiles each workspace root's file set separately and
// returns the primary root's result, the diagnostics of every root merged
// in root order, and the per-root diagnostics keyed by root path. Roots are
// independent projects that merely share a window, so merging their files
// into one compile would leak module derivation and duplicate-declaration
// detection across them; compiling per root keeps each root isolated while
// every root's diagnostics still reach the client through the merged list.
// A root whose file set is unchanged since the last call is served from the
// per-root cache instead of recompiling.
//
// The primary root is rootPath (roots[0]); when no root is set every known
// file forms a single group, preserving single-workspace behavior exactly.
func (w *Workspace) RecompileAll() (*compile.Result, diag.List, map[string]diag.List) {
	w.mu.Lock()
	files := w.collectLocked()
	roots := w.orderedRootsLocked()
	primary := w.primaryRootLocked()
	snapshot := make(map[string]cachedEntry, len(w.cached))
	for root, entry := range w.cached {
		snapshot[root] = entry
	}
	fn := w.compile
	w.mu.Unlock()

	partitions := partitionFilesByRoot(files, roots, primary)

	ordered := make([]string, 0, len(partitions))
	for root := range partitions {
		ordered = append(ordered, root)
	}

	sort.Strings(ordered)

	fresh := make(map[string]cachedEntry, len(partitions))
	dirty := 0

	for _, root := range ordered {
		subset := partitions[root]
		hash := hashFiles(subset)

		if prev, ok := snapshot[root]; ok && prev.result != nil && hash == prev.hash {
			fresh[root] = prev

			continue
		}

		result, diags := fn(subset)
		fresh[root] = cachedEntry{hash: hash, result: result, diags: diags}
		dirty++
	}

	w.mu.Lock()
	for root, entry := range fresh {
		w.cached[root] = entry
	}
	for root := range w.cached {
		if _, ok := fresh[root]; !ok {
			delete(w.cached, root)
		}
	}
	w.compileCount += dirty
	w.mu.Unlock()

	primaryEntry, ok := fresh[primary]
	if !ok && len(ordered) > 0 {
		primaryEntry = fresh[ordered[0]]
	}

	perRoot := make(map[string]diag.List, len(fresh))
	var all diag.List

	for _, root := range ordered {
		perRoot[root] = fresh[root].diags
		all = append(all, fresh[root].diags...)
	}

	return primaryEntry.result, all, perRoot
}

// Recompile runs the lex->parse->resolve pipeline over every known file,
// reusing the previous result when nothing has changed since the last call.
// It compiles each workspace root separately (see RecompileAll) and reports
// the primary root's result with every root's diagnostics merged, so every
// existing caller keeps working while non-primary diagnostics still publish.
func (w *Workspace) Recompile() (*compile.Result, diag.List) {
	result, diags, _ := w.RecompileAll()

	return result, diags
}

// primaryRootLocked reports the root Recompile serves its result for: the
// configured rootPath, falling back to the first tracked root. Callers must
// hold w.mu.
func (w *Workspace) primaryRootLocked() string {
	if w.rootPath != "" {
		return w.rootPath
	}

	if len(w.roots) > 0 {
		return w.roots[0]
	}

	return ""
}

// orderedRootsLocked returns the tracked roots in registration order,
// falling back to the lone rootPath when the list is empty. Callers must
// hold w.mu.
func (w *Workspace) orderedRootsLocked() []string {
	if len(w.roots) > 0 {
		return append([]string(nil), w.roots...)
	}

	if w.rootPath != "" {
		return []string{w.rootPath}
	}

	return nil
}

// partitionFilesByRoot groups files by the longest (most specific) root
// prefix containing them, so nested roots win over their parents. A file
// outside every root (an unsaved buffer with no on-disk counterpart yet)
// falls back to the primary root so it still resolves. Paths and roots are
// compared absolute with filepath separators, so a root /a/b never claims a
// sibling like /a/bc. With no roots every file forms the primary group,
// which keeps single-workspace behavior identical to one merged compile.
func partitionFilesByRoot(files map[string]string, roots []string, primary string) map[string]map[string]string {
	cleanRoots := make([]string, 0, len(roots))

	for _, root := range roots {
		if root == "" {
			continue
		}

		cleanRoots = append(cleanRoots, filepath.Clean(root))
	}

	if len(cleanRoots) == 0 {
		dup := make(map[string]string, len(files))
		for path, content := range files {
			dup[path] = content
		}

		return map[string]map[string]string{primary: dup}
	}

	out := make(map[string]map[string]string, len(cleanRoots)+1)

	for _, root := range cleanRoots {
		out[root] = make(map[string]string)
	}

	if _, ok := out[primary]; !ok && primary != "" {
		out[filepath.Clean(primary)] = make(map[string]string)
		primary = filepath.Clean(primary)
	}

	for path, content := range files {
		out[ownerRootOf(path, cleanRoots, primary)][path] = content
	}

	return out
}

// ownerRootOf assigns path to the longest root prefix containing it, or to
// primary when the path sits outside every root. roots must already be
// filepath.Clean-ed (see partitionFilesByRoot).
func ownerRootOf(path string, roots []string, primary string) string {
	clean := filepath.Clean(path)
	best := ""

	for _, root := range roots {
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			if len(root) > len(best) {
				best = root
			}
		}
	}

	if best != "" {
		return best
	}

	return primary
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

	abs, _ := filepath.Abs(raw) // best-effort mapping for non-conforming clients; Abs only fails when getwd does

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

	abs, _ := filepath.Abs(path) // best-effort mapping; Abs only fails when getwd does

	return uri.File(abs)
}
