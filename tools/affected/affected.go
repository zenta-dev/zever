package main

import (
	"bufio"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// zeverModulePrefix is the module-path prefix shared by every repo module.
const zeverModulePrefix = "github.com/zenta-dev/zever/"

// parseRequires returns the sorted, deduplicated repo-relative module dirs
// required by a go.mod file's content. Both direct and indirect requires
// count: either breaks the importer when the dependency's API changes.
func parseRequires(content string) []string {
	seen := make(map[string]bool)
	inBlock := false
	s := bufio.NewScanner(strings.NewReader(content))
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}
		switch {
		case line == "require (" || strings.HasPrefix(line, "require ("):
			inBlock = true
			continue
		case inBlock && line == ")":
			inBlock = false
			continue
		}
		rest := line
		if strings.HasPrefix(rest, "require ") {
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "require "))
		} else if !inBlock {
			continue
		}
		for _, field := range strings.Fields(rest) {
			field = strings.Trim(field, "\"';")
			if !strings.HasPrefix(field, zeverModulePrefix) {
				continue
			}
			dir := strings.TrimSuffix(strings.TrimPrefix(field, zeverModulePrefix), "/")
			if dir != "" {
				seen[dir] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for dir := range seen {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

// listModules returns every module dir (dir containing go.mod) under root,
// relative without a ./ prefix, sorted. It mirrors the Makefile ALL_MODULES
// find exclusions exactly: ./.git/** and ./examples/external-sms/**.
func listModules(root string) ([]string, error) {
	var mods []string
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
			if rel == ".git" || strings.HasPrefix(rel, ".git/") {
				return filepath.SkipDir
			}
			if rel == "examples/external-sms" || strings.HasPrefix(rel, "examples/external-sms/") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "go.mod" {
			return nil
		}
		mods = append(mods, filepath.ToSlash(filepath.Dir(rel)))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(mods)
	return mods, nil
}

// ownerModule returns the nearest ancestor dir of file (including its own
// dir) that is a known module, or "" when the file has no owning module.
func ownerModule(file string, modules map[string]bool) string {
	dir := path.Dir(path.Clean(filepath.ToSlash(file)))
	for {
		if modules[dir] {
			return dir
		}
		if dir == "." || dir == "/" || dir == "" {
			return ""
		}
		dir = path.Dir(dir)
	}
}

// dependentsClosure returns the sorted union of changed and every module
// that transitively requires one of them. requires maps a module dir to the
// module dirs it requires.
func dependentsClosure(changed []string, requires map[string][]string) []string {
	reverse := make(map[string][]string)
	for mod, deps := range requires {
		for _, dep := range deps {
			reverse[dep] = append(reverse[dep], mod)
		}
	}
	seen := make(map[string]bool, len(changed))
	queue := make([]string, 0, len(changed))
	for _, c := range changed {
		if !seen[c] {
			seen[c] = true
			queue = append(queue, c)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, importer := range reverse[cur] {
			if !seen[importer] {
				seen[importer] = true
				queue = append(queue, importer)
			}
		}
	}
	out := make([]string, 0, len(seen))
	for mod := range seen {
		out = append(out, mod)
	}
	sort.Strings(out)
	return out
}

// chunkGroups slices mods sequentially into at most maxGroups non-empty
// groups. Empty input yields zero groups.
func chunkGroups(mods []string, maxGroups int) [][]string {
	groups := [][]string{}
	if len(mods) == 0 {
		return groups
	}
	if maxGroups < 1 {
		maxGroups = 1
	}
	size := (len(mods) + maxGroups - 1) / maxGroups
	for i := 0; i < len(mods); i += size {
		end := i + size
		if end > len(mods) {
			end = len(mods)
		}
		groups = append(groups, mods[i:end])
	}
	return groups
}

// isDocsPath reports whether a changed file matches the docs-pattern:
// **/*.md, **/*.mdx, docs/**, CHANGELOG.md, CITATION.cff, LICENSE*.
func isDocsPath(file string) bool {
	f := path.Clean(filepath.ToSlash(file))
	base := path.Base(f)
	switch {
	case strings.HasSuffix(f, ".md") || strings.HasSuffix(f, ".mdx"):
		return true
	case f == "docs" || strings.HasPrefix(f, "docs/"):
		return true
	case base == "CHANGELOG.md" || base == "CITATION.cff":
		return true
	case strings.HasPrefix(base, "LICENSE"):
		return true
	}
	return false
}

// isGlobalPath reports whether a changed file forces the ALLModules outcome:
// go.work, go.work.sum, root Makefile, .github/workflows/**, or the
// tools/affected tool itself.
func isGlobalPath(file string) bool {
	f := path.Clean(filepath.ToSlash(file))
	switch {
	case f == "go.work" || f == "go.work.sum" || f == "Makefile":
		return true
	case f == ".github/workflows" || strings.HasPrefix(f, ".github/workflows/"):
		return true
	case f == "tools/affected" || strings.HasPrefix(f, "tools/affected/"):
		return true
	}
	return false
}

// classify maps changed files to (groups, reason) following the contract
// rules in order: none when empty or docs-only, all when any file is global
// or module-less, else owning modules plus transitive reverse dependents.
func classify(changed []string, modules []string, requires map[string][]string, maxGroups int) ([][]string, string) {
	if len(changed) == 0 {
		return [][]string{}, "none"
	}
	docsOnly := true
	for _, f := range changed {
		if !isDocsPath(f) {
			docsOnly = false
			break
		}
	}
	if docsOnly {
		return [][]string{}, "none"
	}
	modSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		modSet[m] = true
	}
	for _, f := range changed {
		if isGlobalPath(f) || ownerModule(f, modSet) == "" {
			return chunkGroups(modules, maxGroups), "all"
		}
	}
	owners := make(map[string]bool)
	for _, f := range changed {
		owners[ownerModule(f, modSet)] = true
	}
	changedMods := make([]string, 0, len(owners))
	for m := range owners {
		changedMods = append(changedMods, m)
	}
	return chunkGroups(dependentsClosure(changedMods, requires), maxGroups), "affected"
}
