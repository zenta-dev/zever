package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// frameworkImportRe matches a quoted framework import path in generated Go
// source (import specs only appear on their own line, optionally aliased).
var frameworkImportRe = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:[\w.]+\s+)?"(github\.com/zenta-dev/zever/[^"]+)"\s*$`)

// goModRequires returns the module path and the set of required module
// paths declared in the go.mod at path. It is a deliberately small line
// parser (no x/mod dependency): enough for require lines and blocks.
func goModRequires(path string) (string, map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}

	defer func() { _ = f.Close() }()

	reqs := map[string]bool{}
	module := ""
	inBlock := false

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch {
		case fields[0] == "module" && len(fields) > 1:
			module = fields[1]
		case fields[0] == "require" && len(fields) > 1 && fields[1] == "(":
			inBlock = true
		case inBlock && fields[0] == ")":
			inBlock = false
		case inBlock:
			reqs[fields[0]] = true
		case fields[0] == "require" && len(fields) > 1:
			reqs[fields[1]] = true
		}
	}

	return module, reqs, sc.Err()
}

// missingGeneratedDeps returns the framework import paths used by the
// generated Go outputs that no module in go.mod provides.
func missingGeneratedDeps(goModPath string, outputs map[string]map[string][]byte) ([]string, error) {
	_, reqs, err := goModRequires(goModPath)
	if err != nil {
		return nil, err
	}

	covered := func(imp string) bool {
		for mod := range reqs {
			if mod == frameworkModulePath {
				continue // the bare root is not a module that provides subpackages
			}

			if imp == mod || strings.HasPrefix(imp, mod+"/") {
				return true
			}
		}

		return false
	}

	missing := map[string]bool{}

	for _, files := range outputs {
		for name, content := range files {
			if !strings.HasSuffix(name, ".go") {
				continue
			}

			for _, m := range frameworkImportRe.FindAllSubmatch(content, -1) {
				if imp := string(m[1]); !covered(imp) {
					missing[imp] = true
				}
			}
		}
	}

	out := make([]string, 0, len(missing))
	for imp := range missing {
		out = append(out, imp)
	}

	sort.Strings(out)

	return out, nil
}

// ensureGeneratedDeps checks that the module rooted at dir requires every
// framework package the freshly generated Go outputs import. When some are
// missing it runs `go mod tidy` in dir (or, with noTidy, only prints an
// actionable hint to w). It does nothing when dir has no go.mod or outDir
// lies outside the module. A tidy failure is reported but never fails the
// compile: the generated files are already written.
func ensureGeneratedDeps(dir, outDir string, outputs map[string]map[string][]byte, noTidy bool, w io.Writer) error {
	goMod := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(goMod); err != nil {
		return nil //nolint:nilerr // no go.mod: nothing to reconcile
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil //nolint:nilerr // best-effort check
	}

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return nil //nolint:nilerr // best-effort check
	}

	if rel, relErr := filepath.Rel(absDir, absOut); relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil //nolint:nilerr // outputs live outside this module
	}

	missing, err := missingGeneratedDeps(goMod, outputs)
	if err != nil || len(missing) == 0 {
		return nil //nolint:nilerr // unreadable go.mod: best-effort, leave it to go build
	}

	list := strings.Join(missing, ", ")

	if noTidy {
		_, _ = fmt.Fprintln(w, formatHint(fmt.Sprintf("generated code imports %s, which go.mod does not require; run 'go mod tidy'", list)))

		return nil
	}

	if tidyErr := runGoModTidyFunc("zever compile", dir); tidyErr != nil {
		_, _ = fmt.Fprintln(w, formatHint(fmt.Sprintf("generated code imports %s, which go.mod does not require; run 'go mod tidy' (automatic tidy failed: %v)", list, tidyErr)))
	}

	return nil
}
