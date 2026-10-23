package main

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/zenta-dev/zever/shared/codec"
)

// zeverPrefix is the module-path prefix shared by every repo module.
const zeverPrefix = "github.com/zenta-dev/zever/"

// ErrGoModEdit reports a failed `go mod edit -json` invocation.
var ErrGoModEdit = errors.New("modgraph: go mod edit failed")

// ErrParseGoMod reports unparsable `go mod edit -json` output.
var ErrParseGoMod = errors.New("modgraph: parse go mod edit output failed")

// ErrEmptyModulePath reports `go mod edit -json` output with no module path.
var ErrEmptyModulePath = errors.New("modgraph: go mod edit output has empty module path")

// drift lists the require+replace entries a module's go.mod lacks, plus
// the replace directives a no-replace module must not carry.
type drift struct {
	Require   []string
	Replace   []string
	Forbidden []string
}

// noReplaceModules lists module dirs that must commit no intra-repo
// `replace` directives. These are the `go install <module>@version`
// targets: the go command refuses to install a main package whose go.mod
// holds replace directives. Local development resolves their siblings
// through go.work; consumers resolve them from the pinned requires.
var noReplaceModules = map[string]bool{
	"cmd/zever": true,
}

// tagPathExceptions lists module dirs whose module path deliberately
// breaks the tag rule: out-of-tree consumer proofs kept outside go.work.
// docs/examples uses a dashed path so it resolves like a published module,
// and examples/external-sms lives under another org entirely.
var tagPathExceptions = map[string]string{
	"docs/examples":         "github.com/zenta-dev/zever/docs-examples",
	"examples/external-sms": "github.com/example/zever-sms",
}

// tagPathDrift returns moddir -> actual module path for every module whose
// path cannot produce its release tag via the tools/tag-release.sh rule
// (tag `<module-path-suffix>/vX.Y.Z`). Conformity means the module path is
// exactly zeverPrefix plus the repo-relative dir; the root dir (no root
// module: the script tags nothing there) and the tagPathExceptions are
// skipped.
func tagPathDrift(mods map[string]string) map[string]string {
	dirs := make([]string, 0, len(mods))
	byDir := make(map[string]string, len(mods))
	for path, dir := range mods {
		dirs = append(dirs, dir)
		byDir[dir] = path
	}
	sort.Strings(dirs)
	out := make(map[string]string)
	for _, dir := range dirs {
		path := byDir[dir]
		if dir == "." {
			continue
		}
		if want, ok := tagPathExceptions[dir]; ok && path == want {
			continue
		}
		if path != zeverPrefix+dir {
			out[dir] = path
		}
	}
	return out
}

// scanImports returns the set of intra-repo import paths declared by real
// import statements in src. Only parser ImportSpecs count, so paths inside
// raw-string templates, string constants, and comments never count, while
// block, single-line, aliased, blank, and dot imports always do.
// Unparsable source yields an empty set.
func scanImports(src string) map[string]bool {
	found := make(map[string]bool)
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, parser.ImportsOnly)
	if err != nil {
		return found
	}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, zeverPrefix) {
			found[path] = true
		}
	}
	return found
}

// provider returns the longest module path in mods that imp belongs to
// (exact match or path+"/" prefix), or "" when no module provides imp.
func provider(imp string, mods map[string]string) string {
	best := ""
	for path := range mods {
		if imp == path || strings.HasPrefix(imp, path+"/") {
			if len(path) > len(best) {
				best = path
			}
		}
	}
	return best
}

// modEditJSON mirrors the `go mod edit -json` Module stanza.
type modEditJSON struct {
	Module struct {
		Path string `json:"Path"`
	} `json:"Module"`
}

// skipModuleDir reports whether the walk must not descend into the
// directory at repo-relative slash path rel. The excluded set is the one
// tools/tag-release.sh filters out of its `find`: the git metadata dir,
// nested worktrees and the agent harness checkout dir, plus any vendored
// node_modules copy. A nested worktree is a full checkout of the repo, so
// its modules are duplicates of the repo's own: counting them made the
// module graph see a module that does not exist in the tagged tree (tag
// path rule) and made the pin gate read a worktree's go.mod. Keeping
// both tools on the same exclusion set keeps the tag list and the module
// list in agreement. The excluded names are matched at ANY depth: a
// worktree nested under a module dir (tools/.worktrees/wt) is a checkout
// too, and the tagger skips it as well. testdata modules are NOT excluded:
// they are real modules of the tree and their pins are checked like every
// other one (the tagger does not publish them).
func skipModuleDir(rel string) bool {
	if rel == "." {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		switch part {
		case ".git", ".worktrees", ".claude", "node_modules":
			return true
		}
	}
	return false
}

// findModules returns import path -> repo-relative slash dir for every
// directory under root holding a go.mod. Discovery runs `go mod edit -json`
// per dir like the Python find_modules. Nested worktrees, harness checkouts
// and vendored copies are skipped (see skipModuleDir).
func findModules(ctx context.Context, root string) (map[string]string, error) {
	mods := make(map[string]string)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipModuleDir(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		cmd := exec.CommandContext(ctx, "go", "mod", "edit", "-json", "go.mod")
		cmd.Dir = filepath.Dir(p)
		out, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("modgraph: go mod edit -json in %s: %w", filepath.Dir(p), errors.Join(ErrGoModEdit, err))
		}
		var v modEditJSON
		if err := codec.Unmarshal(out, &v); err != nil {
			return fmt.Errorf("modgraph: parse go mod edit output in %s: %w", filepath.Dir(p), errors.Join(ErrParseGoMod, err))
		}
		if v.Module.Path == "" {
			return fmt.Errorf("modgraph: go mod edit output in %s has empty module path: %w", filepath.Dir(p), ErrEmptyModulePath)
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		mods[v.Module.Path] = dir
		return nil
	})
	if err != nil {
		return nil, err
	}
	return mods, nil
}

// hasGoMod reports whether dir holds a go.mod file.
func hasGoMod(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil && !fi.IsDir()
}

// localImports returns all intra-repo import paths in .go files under
// moddir, excluding nested-module subtrees and the skipped directories
// (nested worktrees, harness checkouts, vendored copies).
func localImports(root, moddir string) (map[string]bool, error) {
	base := filepath.Join(root, filepath.FromSlash(moddir))
	found := make(map[string]bool)
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if skipModuleDir(rel) {
				return filepath.SkipDir
			}
			if rel != moddir && hasGoMod(p) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		content, err := os.ReadFile(p) //nolint:gosec // G122: p comes from walking the repository root; modgraph is a dev tool, not a production service
		if err != nil {
			return err
		}
		for imp := range scanImports(string(content)) {
			found[imp] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

var (
	singleRequire = regexp.MustCompile(`require\s+(github\.com/zenta-dev/zever/\S+)\s+\S+`)
	blockRequire  = regexp.MustCompile(`(?s)require\s*\((.*?)\)`)
	singleReplace = regexp.MustCompile(`replace\s+(github\.com/zenta-dev/zever/\S+)\s+=>`)
	blockReplace  = regexp.MustCompile(`(?s)replace\s*\((.*?)\)`)
	versionHeader = regexp.MustCompile(`^## \[(v\d+\.\d+\.\d+)\]`)
)

// parseRequires returns the local zever paths required by a go.mod's
// content, covering single-line and block require forms.
func parseRequires(text string) map[string]bool {
	reqs := make(map[string]bool)
	for _, m := range singleRequire.FindAllStringSubmatch(text, -1) {
		reqs[m[1]] = true
	}
	for _, m := range blockRequire.FindAllStringSubmatch(text, -1) {
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, zeverPrefix) {
				if fields := strings.Fields(line); len(fields) > 0 {
					reqs[fields[0]] = true
				}
			}
		}
	}
	return reqs
}

// parseReplaces returns the local zever paths replaced by a go.mod's
// content, covering single-line and block replace forms.
func parseReplaces(text string) map[string]bool {
	reps := make(map[string]bool)
	for _, m := range singleReplace.FindAllStringSubmatch(text, -1) {
		reps[m[1]] = true
	}
	for _, m := range blockReplace.FindAllStringSubmatch(text, -1) {
		for _, line := range strings.Split(m[1], "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, zeverPrefix) {
				if fields := strings.Fields(line); len(fields) > 0 {
					reps[fields[0]] = true
				}
			}
		}
	}
	return reps
}

// gomodRequires returns the local zever paths required by moddir/go.mod.
func gomodRequires(root, moddir string) map[string]bool {
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(moddir), "go.mod"))
	if err != nil {
		return make(map[string]bool)
	}
	return parseRequires(string(content))
}

// gomodReplaces returns the local zever paths replaced by moddir/go.mod.
func gomodReplaces(root, moddir string) map[string]bool {
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(moddir), "go.mod"))
	if err != nil {
		return make(map[string]bool)
	}
	return parseReplaces(string(content))
}

// neededClosure returns the direct intra-repo module dependencies of one
// module: providers of its own imports, excluding itself. Deliberately
// direct-only: `go mod tidy` drops requires nothing imports, so a
// transitive closure here would fight tidy forever.
func neededClosure(root, path, moddir string, mods map[string]string) (map[string]bool, error) {
	imports, err := localImports(root, moddir)
	if err != nil {
		return nil, err
	}
	needed := make(map[string]bool)
	for imp := range imports {
		if prov := provider(imp, mods); prov != "" && prov != path {
			needed[prov] = true
		}
	}
	return needed, nil
}

// lockstepVersion returns the first `## [vX.Y.Z]` header in the root
// CHANGELOG.md, or v0.0.0 when none is found.
func lockstepVersion(root string) string {
	content, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		return "v0.0.0"
	}
	for _, line := range strings.Split(string(content), "\n") {
		if m := versionHeader.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			return m[1]
		}
	}
	return "v0.0.0"
}

// checkModules returns per-module-dir missing require+replace entries.
// The root module (dir ".") is skipped.
func checkModules(root string, mods map[string]string) map[string]drift {
	paths := make([]string, 0, len(mods))
	for path := range mods {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	out := make(map[string]drift)
	for _, path := range paths {
		moddir := mods[path]
		if moddir == "." {
			continue
		}
		needed, err := neededClosure(root, path, moddir, mods)
		if err != nil || (len(needed) == 0 && !noReplaceModules[moddir]) {
			continue
		}
		reqs := gomodRequires(root, moddir)
		reps := gomodReplaces(root, moddir)
		var missingReq, missingRep, forbidden []string
		for dep := range needed {
			if !reqs[dep] {
				missingReq = append(missingReq, dep)
			}
			if !noReplaceModules[moddir] && !reps[dep] {
				missingRep = append(missingRep, dep)
			}
		}
		if noReplaceModules[moddir] {
			for dep := range reps {
				forbidden = append(forbidden, dep)
			}
		}
		if len(missingReq) > 0 || len(missingRep) > 0 || len(forbidden) > 0 {
			sort.Strings(missingReq)
			sort.Strings(missingRep)
			sort.Strings(forbidden)
			out[moddir] = drift{Require: missingReq, Replace: missingRep, Forbidden: forbidden}
		}
	}
	return out
}

// fixModule adds require@ver + relative replace entries for every dep of
// one module via `go mod edit`.
func fixModule(ctx context.Context, root, moddir string, deps []string, mods map[string]string, ver string) error {
	full := filepath.Join(root, filepath.FromSlash(moddir))
	for _, dep := range deps {
		rel, err := filepath.Rel(full, filepath.Join(root, filepath.FromSlash(mods[dep])))
		if err != nil {
			return err
		}
		args := []string{"mod", "edit", "-require=" + dep + "@" + ver}
		if noReplaceModules[moddir] {
			args = append(args, "-dropreplace="+dep)
		} else {
			args = append(args, "-replace="+dep+"="+filepath.ToSlash(rel))
		}
		args = append(args, "go.mod")
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = full
		if out, err := cmd.CombinedOutput(); err != nil {
			return &fixError{dep: dep, output: string(out), err: err}
		}
	}
	return nil
}

// dropReplaces removes every intra-repo replace directive from a no-replace
// module's go.mod via `go mod edit`.
func dropReplaces(ctx context.Context, root, moddir string) error {
	reps := gomodReplaces(root, moddir)
	if len(reps) == 0 {
		return nil
	}
	args := []string{"mod", "edit"}
	for _, dep := range sortedMapKeys(reps) {
		args = append(args, "-dropreplace="+dep)
	}
	args = append(args, "go.mod")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = filepath.Join(root, filepath.FromSlash(moddir))
	if out, err := cmd.CombinedOutput(); err != nil {
		return &fixError{dep: "(replace directives)", output: string(out), err: err}
	}
	return nil
}

// sortedMapKeys returns the sorted keys of set.
func sortedMapKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fixError records a failed `go mod edit` invocation for one dependency.
type fixError struct {
	dep    string
	output string
	err    error
}

// Error returns the failure description.
func (e *fixError) Error() string {
	return "modgraph: go mod edit for " + e.dep + ": " + e.err.Error() + ": " + e.output
}

// Unwrap returns the underlying command error.
func (e *fixError) Unwrap() error { return e.err }
