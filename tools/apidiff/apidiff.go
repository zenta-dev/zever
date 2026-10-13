package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/exp/apidiff"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

// coreModules is the Core-tier API surface the gate protects: every module
// whose exported API other modules and consumer projects compile against.
// Adapter modules are excluded by design: adapters register behind the
// core/<battery> interfaces, so their API is not a compatibility surface.
var coreModules = []string{
	"config",
	"container",
	"core/auth",
	"core/cache",
	"core/crypto",
	"core/db",
	"core/password",
	"core/queue",
	"core/router",
	"core/secrets",
	"core/session",
	"dsl",
	"orm",
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("apidiff", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rootFlag := fs.String("root", "", "repo root (default: git toplevel of cwd)")
	modulesFlag := fs.String("modules", "", "comma-separated module dirs (default: Core tier)")
	changelogFlag := fs.String("changelog", "", "changelog path (default: <root>/CHANGELOG.md)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	root := *rootFlag
	if root == "" {
		var err error
		root, err = gitTopLevel(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "apidiff: git rev-parse --show-toplevel: %v\n", err)
			return 1
		}
	}
	modules := coreModules
	if *modulesFlag != "" {
		modules = strings.Split(*modulesFlag, ",")
	}
	changelog := *changelogFlag
	if changelog == "" {
		changelog = filepath.Join(root, "CHANGELOG.md")
	}

	type failure struct {
		module string
		tag    string
		report string
	}
	var failures []failure
	for _, dir := range modules {
		dir = strings.TrimSpace(dir)
		tag, err := latestTag(ctx, root, dir)
		if err != nil {
			fmt.Fprintf(stderr, "apidiff: %s: %v; skipping\n", dir, err)
			continue
		}
		report, err := compareModule(ctx, root, dir, tag)
		if err != nil {
			fmt.Fprintf(stderr, "apidiff: %s: compare against %s: %v\n", dir, tag, err)
			return 1
		}
		if report != "" {
			failures = append(failures, failure{module: dir, tag: tag, report: report})
		} else {
			fmt.Fprintf(stdout, "apidiff: %s compatible with %s\n", dir, tag)
		}
	}

	if len(failures) == 0 {
		fmt.Fprintf(stdout, "apidiff: all checked modules compatible\n")
		return 0
	}

	for _, f := range failures {
		fmt.Fprintf(stdout, "apidiff: %s breaks %s:\n%s\n", f.module, f.tag, f.report)
	}
	raw, err := os.ReadFile(changelog)
	if err != nil {
		fmt.Fprintf(stderr, "apidiff: read %s: %v\n", changelog, err)
		return 1
	}
	if !unreleasedHasBreaking(string(raw)) {
		fmt.Fprintf(stdout, "apidiff: incompatible API changes without a **Breaking:** line under ## [Unreleased] in %s\n", changelog)
		return 1
	}
	fmt.Fprintf(stdout, "apidiff: incompatible changes documented under ## [Unreleased]\n")
	return 0
}

func gitTopLevel(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// latestTag returns the highest <dir>/v* tag, e.g. "core/cache/v0.6.1".
func latestTag(ctx context.Context, root, dir string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "-C", root, "tag", "--list", dir+"/v*", "--sort=-v:refname").Output()
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(out), "\n")
	if line == "" {
		return "", fmt.Errorf("apidiff: no tag matching %s/v*", dir)
	}
	return line, nil
}

// compareModule diffs the exported API of dir at tag against the working
// tree and returns the rendered incompatible-changes report (empty when
// compatible).
func compareModule(ctx context.Context, root, dir, tag string) (string, error) {
	base, err := os.MkdirTemp("", "apidiff-base-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(base)

	archive, err := exec.CommandContext(ctx, "git", "-C", root, "archive", tag).Output()
	if err != nil {
		return "", fmt.Errorf("git archive %s: %w", tag, err)
	}
	untar := exec.CommandContext(ctx, "tar", "-x", "-C", base)
	untar.Stdin = strings.NewReader(string(archive))
	if out, untarErr := untar.CombinedOutput(); untarErr != nil {
		return "", fmt.Errorf("extract %s\n%s: %w", tag, out, untarErr)
	}

	importPath, err := goModPath(filepath.Join(root, dir, "go.mod"))
	if err != nil {
		return "", err
	}
	oldMod, err := loadModule(filepath.Join(base, dir), importPath)
	if err != nil {
		return "", fmt.Errorf("load base %s: %w", tag, err)
	}
	newMod, err := loadModule(filepath.Join(root, dir), importPath)
	if err != nil {
		return "", fmt.Errorf("load working tree: %w", err)
	}
	report := apidiff.ModuleChanges(oldMod, newMod)
	var sb strings.Builder
	if err := report.TextIncompatible(&sb, true); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// goModPath reads the module path from a go.mod file.
func goModPath(goMod string) (string, error) {
	raw, err := os.ReadFile(goMod)
	if err != nil {
		return "", err
	}
	f, err := modfile.Parse(goMod, raw, nil)
	if err != nil {
		return "", err
	}
	if f.Module == nil || f.Module.Mod.Path == "" {
		return "", fmt.Errorf("apidiff: no module line in %s", goMod)
	}
	return f.Module.Mod.Path, nil
}

// loadModule type-checks every non-internal package of importPath rooted at
// dir (a module dir, loaded in module mode so workspace replaces never leak
// the other side of the comparison in).
func loadModule(dir, importPath string) (*apidiff.Module, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedModule,
		Dir:  dir,
		Env:  append(os.Environ(), "GOWORK=off"),
	}
	loaded, err := packages.Load(cfg, importPath+"/...")
	if err != nil {
		return nil, err
	}
	if len(loaded) == 0 {
		return nil, fmt.Errorf("apidiff: found no packages for module %s", importPath)
	}
	var pkgs []*packages.Package
	var errs []error
	for _, p := range loaded {
		if len(p.Errors) > 0 {
			for _, e := range p.Errors {
				errs = append(errs, e)
			}
			continue
		}
		if isInternalPackage(p.PkgPath, importPath) {
			continue
		}
		pkgs = append(pkgs, p)
	}
	if len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return nil, fmt.Errorf("load %s: %s", importPath, strings.Join(msgs, "\n"))
	}
	mod := &apidiff.Module{Path: importPath}
	for _, p := range pkgs {
		mod.Packages = append(mod.Packages, p.Types)
	}
	return mod, nil
}

func isInternalPackage(pkgPath, modulePath string) bool {
	rel := strings.TrimPrefix(pkgPath, modulePath)
	switch {
	case strings.HasSuffix(rel, "/internal"):
		return true
	case strings.Contains(rel, "/internal/"):
		return true
	case rel == "internal":
		return true
	case strings.HasPrefix(rel, "internal/"):
		return true
	}
	return false
}

// unreleasedHasBreaking reports whether body carries a **Breaking:** line
// inside the ## [Unreleased] section (stopping at the next ## [x] header).
func unreleasedHasBreaking(body string) bool {
	lines := strings.Split(body, "\n")
	inUnreleased := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inUnreleased = trimmed == "## [Unreleased]"
			continue
		}
		if inUnreleased && strings.Contains(line, "**Breaking:**") {
			return true
		}
	}
	return false
}
