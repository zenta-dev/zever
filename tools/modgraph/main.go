// Command modgraph ensures every module's go.mod requires+replaces what
// it directly imports. Go ignores replace directives in dependency
// go.mods, so each module must repeat require+replace for every
// intra-repo module it directly imports. Direct-only is deliberate:
// `go mod tidy` drops requires nothing imports, so a transitive closure
// here would fight tidy forever.
//
// Usage: modgraph [--check] [--version=vX.Y.Z] [--root DIR]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("modgraph", flag.ContinueOnError)
	fs.SetOutput(stderr)
	check := fs.Bool("check", false, "report drift without changing files (exit 1 when drifted)")
	version := fs.String("version", "", "version for new requires (default: first ## [vX.Y.Z] in CHANGELOG.md)")
	rootFlag := fs.String("root", "", "repo root (default: source tree root)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	root := *rootFlag
	if root == "" {
		root = defaultRoot()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintf(stderr, "modgraph: %v\n", err)
		return 1
	}
	root = abs

	mods, err := findModules(root)
	if err != nil {
		fmt.Fprintf(stderr, "modgraph: list modules: %v\n", err)
		return 1
	}
	if *check {
		return runCheck(root, mods, stdout)
	}
	ver := *version
	if ver == "" {
		ver = lockstepVersion(root)
	}
	return runFix(root, mods, ver, stdout, stderr)
}

// defaultRoot returns the repo root derived from this source file's
// location (two dirs above tools/modgraph), falling back to the working
// directory when that cannot be determined.
func defaultRoot() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		if abs, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", "..")); err == nil {
			return abs
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// runCheck prints the drift listing and returns 1 when any module lacks
// entries, 0 when clean.
func runCheck(root string, mods map[string]string, stdout *os.File) int {
	drift := checkModules(root, mods)
	if len(drift) == 0 {
		fmt.Fprintf(stdout, "modules: %d; module-graph clean\n", len(mods))
		return 0
	}
	fmt.Fprintf(stdout, "modules: %d; drift in %d module(s):\n", len(mods), len(drift))
	for _, moddir := range sortedKeys(drift) {
		missing := drift[moddir]
		if len(missing.Require) > 0 {
			fmt.Fprintf(stdout, "%s: missing require: %s\n", moddir, joinAll(missing.Require))
		}
		if len(missing.Replace) > 0 {
			fmt.Fprintf(stdout, "%s: missing replace: %s\n", moddir, joinAll(missing.Replace))
		}
	}
	return 1
}

// runFix adds missing require+replace pairs per module via `go mod edit`.
func runFix(root string, mods map[string]string, ver string, stdout, stderr *os.File) int {
	fmt.Fprintf(stdout, "modules: %d\n", len(mods))
	paths := make([]string, 0, len(mods))
	for path := range mods {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		moddir := mods[path]
		if moddir == "." {
			continue
		}
		needed, err := neededClosure(root, path, moddir, mods)
		if err != nil {
			fmt.Fprintf(stderr, "modgraph: %s: %v\n", moddir, err)
			return 1
		}
		if len(needed) == 0 {
			continue
		}
		deps := make([]string, 0, len(needed))
		for dep := range needed {
			deps = append(deps, dep)
		}
		sort.Strings(deps)
		if err := fixModule(root, moddir, deps, mods, ver); err != nil {
			fmt.Fprintf(stderr, "modgraph: %s: %v\n", moddir, err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: +%d\n", moddir, len(needed))
	}
	return 0
}

// sortedKeys returns the sorted module dirs with drift.
func sortedKeys(d map[string]drift) []string {
	out := make([]string, 0, len(d))
	for k := range d {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// joinAll joins entries with single spaces.
func joinAll(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}
