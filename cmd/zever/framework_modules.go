package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// frameworkRuntimeRoots are the checkout directories whose modules form the
// framework's runtime surface: one module per battery (core/<battery>), one
// per adapter (adapters/<battery>/<adapter>) and the SDK-free helpers
// (shared/<helper>). Every one of them is a module the scaffolded project
// may end up depending on -- directly, through the container/serve module
// graph, or later through `zever add` -- so the local-checkout replace set
// is rooted at them instead of at a hand-written list of module names.
//
// Rooting at directories (not module paths) is what keeps the set correct as
// the framework grows: a new battery, adapter or helper lands in one of
// these directories and is covered without touching the CLI.
var frameworkRuntimeRoots = []string{"adapters", "core", "shared"}

// scanFrameworkModules walks root and returns every framework module in the
// checkout, keyed by module path, with its repo-relative directory.
//
// Modules outside the framework's own module path (examples/external-sms
// lives under another org) and fixture modules under testdata directories
// are skipped: neither is part of the published release line a scaffolded
// project resolves against.
func scanFrameworkModules(root string) (map[string]string, error) {
	mods := make(map[string]string)

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "testdata":
				return filepath.SkipDir
			}

			return nil
		}

		if d.Name() != "go.mod" {
			return nil
		}

		path := modulePathOf(p)
		if path == "" || !strings.HasPrefix(path, frameworkModulePath+"/") {
			return nil
		}

		rel, rerr := filepath.Rel(root, filepath.Dir(p))
		if rerr != nil {
			return fmt.Errorf("locate %q in %q: %w", path, root, rerr)
		}

		mods[path] = filepath.ToSlash(rel)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan framework modules in %q: %w", root, err)
	}

	return mods, nil
}

// frameworkRequirePath returns the framework module path named by one
// require entry's fields, and whether it names one at all.
func frameworkRequirePath(fields string) (string, bool) {
	parts := strings.Fields(fields)
	if len(parts) == 0 {
		return "", false
	}

	path := strings.Trim(parts[0], `"`)
	if !strings.HasPrefix(path, frameworkModulePath+"/") {
		return "", false
	}

	return path, true
}

// frameworkModuleRequires returns the framework module paths the go.mod at
// path requires, covering both the single-line and the block require form.
func frameworkModuleRequires(gomodPath string) ([]string, error) {
	data, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", gomodPath, err)
	}

	var out []string

	inBlock := false

	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if inBlock {
			if trimmed == ")" {
				inBlock = false

				continue
			}

			if path, ok := frameworkRequirePath(trimmed); ok {
				out = append(out, path)
			}

			continue
		}

		if trimmed == "require (" || trimmed == "require(" {
			inBlock = true

			continue
		}

		if rest, ok := strings.CutPrefix(trimmed, "require "); ok {
			if path, ok := frameworkRequirePath(rest); ok {
				out = append(out, path)
			}
		}
	}

	return out, nil
}

// frameworkRuntimeModules returns every framework module a scaffolded
// project can reach in the checkout at root: the given roots (the modules
// the scaffold requires directly) plus the runtime roots (the batteries,
// the adapters and the shared helpers) plus every module those require
// inside the repository -- the top-level runtime modules and the DSL
// compiler module among them.
//
// The result is the module graph, not a name list: a module the framework
// adds later is covered as soon as the checkout ships it, and nothing
// outside the repository is ever in the set. The paths come back sorted, so
// the generated go.mod stays byte-stable.
func frameworkRuntimeModules(root string, roots []string) ([]string, error) {
	mods, err := scanFrameworkModules(root)
	if err != nil {
		return nil, err
	}

	queue := append([]string{}, roots...)

	for path, dir := range mods {
		for _, prefix := range frameworkRuntimeRoots {
			if strings.HasPrefix(dir, prefix+"/") {
				queue = append(queue, path)

				break
			}
		}
	}

	seen := make(map[string]bool, len(mods))

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		if seen[cur] {
			continue
		}

		dir, ok := mods[cur]
		if !ok {
			continue
		}

		seen[cur] = true

		reqs, rerr := frameworkModuleRequires(filepath.Join(root, dir, "go.mod"))
		if rerr != nil {
			return nil, rerr
		}

		for _, dep := range reqs {
			if !seen[dep] {
				queue = append(queue, dep)
			}
		}
	}

	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}

	sort.Strings(out)

	return out, nil
}
