package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"strconv"
	"strings"
)

// configParserModules are the module roots of the YAML and TOML parsers the
// framework must not pull into shipped code.
//
// Config is env-only since v0.8.0: there is no config file to parse, so a
// parser in a non-test file is either dead weight or the first step back
// to file-based configuration. A test file may parse YAML (the CLI tests
// parse the GitHub-Actions workflow the scaffolder writes), because a test
// reads a file it does not ship.
//
// The list is a deny-list of module roots, matched as a path prefix: a
// package of a denied module is denied too.
var configParserModules = []string{
	"go.yaml.in/yaml/v3",
	"go.yaml.in/yaml/v4",
	"gopkg.in/yaml.v2",
	"gopkg.in/yaml.v3",
	"gopkg.in/yaml.v4",
	"github.com/gopkg.in/yaml.v2",
	"github.com/gopkg.in/yaml.v3",
	"github.com/BurntSushi/toml",
	"github.com/pelletier/go-toml",
}

// configParserImport reports whether the import path names a YAML or TOML
// parser package, and the module root it belongs to.
func configParserImport(path string) (string, bool) {
	for _, mod := range configParserModules {
		if path == mod || strings.HasPrefix(path, mod+"/") {
			return mod, true
		}
	}

	return "", false
}

// loadNoConfigParserAllowlist parses no_config_parser_allowlist.txt in
// dir: one entry per line, either a bare repo-relative path or path:line
// (1-based). Blank lines and # comments are ignored. A missing file yields
// an empty allowlist: the rule stays in force, it only has no exception.
func loadNoConfigParserAllowlist(dir string) (allowlist, error) {
	f, closeFile, err := openContained(dir, "no_config_parser_allowlist.txt")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return allowlist{}, nil
		}

		return nil, fmt.Errorf("tagcheck: open no-config-parser allowlist: %w", err)
	}
	defer closeFile()

	a := allowlist{}

	parseErr := eachDataLine(f, "no_config_parser_allowlist.txt", func(line string) error {
		a[strings.TrimSpace(line)] = true
		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return a, nil
}

// checkConfigParserImports reports every import of a YAML or TOML parser in
// a non-test, non-generated Go file.
//
// The rule exists because a direct requirement is not a promise: a
// test-only dependency is a direct requirement (go mod tidy keeps it
// direct, because a _test.go file is a package in the main module), so
// nothing stops a contributor from importing the parser in shipped code.
// The import is what makes the dependency load-bearing, so the import is
// what the rule watches.
func checkConfigParserImports(rel string, fset *token.FileSet, f *ast.File, r rules) []violation {
	var vs []violation

	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}

		mod, ok := configParserImport(path)
		if !ok {
			continue
		}

		line := fset.Position(imp.Pos()).Line
		if r.parserAllowed[rel] || r.parserAllowed[fmt.Sprintf("%s:%d", rel, line)] {
			continue
		}

		vs = append(vs, violation{
			path: rel,
			line: line,
			rule: "no-config-parser",
			msg:  fmt.Sprintf("imports %s from the denied module %s; v0.8.0 is env-only and the framework parses no config file, so a YAML or TOML parser belongs in a test", path, mod),
		})
	}

	return vs
}
