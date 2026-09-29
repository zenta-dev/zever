package main

import (
	"encoding/json"
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
)

// zeverPrefix is the module-path prefix shared by every repo module.
const zeverPrefix = "github.com/zenta-dev/zever/"

// drift lists the require+replace entries a module's go.mod lacks.
type drift struct {
	Require []string
	Replace []string
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

// findModules returns import path -> repo-relative slash dir for every
// directory under root holding a go.mod. Discovery runs `go mod edit -json`
// per dir like the Python find_modules. .git and node_modules subtrees are
// skipped.
func findModules(root string) (map[string]string, error) {
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
			if rel == ".git" || strings.HasPrefix(rel, ".git/") ||
				rel == "node_modules" || strings.HasPrefix(rel, "node_modules/") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		cmd := exec.Command("go", "mod", "edit", "-json", "go.mod")
		cmd.Dir = filepath.Dir(p)
		out, err := cmd.Output()
		if err != nil {
			return nil
		}
		var v modEditJSON
		if err := json.Unmarshal(out, &v); err != nil || v.Module.Path == "" {
			return nil
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
// moddir, excluding nested-module subtrees.
func localImports(root, moddir string, mods map[string]string) (map[string]bool, error) {
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
			if rel != moddir && hasGoMod(p) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		content, err := os.ReadFile(p)
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
	imports, err := localImports(root, moddir, mods)
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
		if err != nil || len(needed) == 0 {
			continue
		}
		reqs := gomodRequires(root, moddir)
		reps := gomodReplaces(root, moddir)
		var missingReq, missingRep []string
		for dep := range needed {
			if !reqs[dep] {
				missingReq = append(missingReq, dep)
			}
			if !reps[dep] {
				missingRep = append(missingRep, dep)
			}
		}
		if len(missingReq) > 0 || len(missingRep) > 0 {
			sort.Strings(missingReq)
			sort.Strings(missingRep)
			out[moddir] = drift{Require: missingReq, Replace: missingRep}
		}
	}
	return out
}

// fixModule adds require@ver + relative replace entries for every dep of
// one module via `go mod edit`.
func fixModule(root, moddir string, deps []string, mods map[string]string, ver string) error {
	full := filepath.Join(root, filepath.FromSlash(moddir))
	for _, dep := range deps {
		rel, err := filepath.Rel(full, filepath.Join(root, filepath.FromSlash(mods[dep])))
		if err != nil {
			return err
		}
		cmd := exec.Command("go", "mod", "edit",
			"-require="+dep+"@"+ver,
			"-replace="+dep+"="+filepath.ToSlash(rel),
			"go.mod")
		cmd.Dir = full
		if out, err := cmd.CombinedOutput(); err != nil {
			return &fixError{dep: dep, output: string(out), err: err}
		}
	}
	return nil
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
