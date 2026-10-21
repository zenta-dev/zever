package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"regexp"
	"strconv"
	"strings"
)

// legacyConfigPrefix is the file-name prefix of the config files the
// v0.8.0 env-only break removed. The guard spells the retired names from
// this prefix and the extensions in newLegacyConfigRule, so the guard
// itself never names a retired file: the rule applies to the guard's own
// source like to every other non-test Go file.
const legacyConfigPrefix = "zever."

// legacyConfigRule holds the retired config file names and the pattern
// that finds one of them inside a string.
type legacyConfigRule struct {
	names   []string
	pattern *regexp.Regexp
}

// newLegacyConfigRule builds the rule data: the retired file names and the
// case-insensitive pattern that matches one of them inside a string.
func newLegacyConfigRule() *legacyConfigRule {
	exts := []string{"json", "yaml", "yml", "toml"}

	names := make([]string, 0, len(exts))
	for _, ext := range exts {
		names = append(names, legacyConfigPrefix+ext)
	}

	pattern := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(legacyConfigPrefix) + `(?:` + strings.Join(exts, "|") + `)`)

	return &legacyConfigRule{names: names, pattern: pattern}
}

// match returns the retired config file name that value names, and whether
// it names one at all. The match is case-insensitive and must sit on a name
// boundary: the characters around it must not extend the name, so a
// "zever.schema.json" file (a different file) is not a hit.
func (r *legacyConfigRule) match(value string) (string, bool) {
	start, end, ok := r.matchOffset(value)
	if !ok {
		return "", false
	}

	return value[start:end], true
}

// matchOffset returns the byte range of the retired config file name that
// value names, and whether it names one at all. The match is
// case-insensitive and must sit on a name boundary: the characters around it
// must not extend the name, so a "zever.schema.json" file (a different
// file) is not a hit.
func (r *legacyConfigRule) matchOffset(value string) (int, int, bool) {
	for _, loc := range r.pattern.FindAllStringIndex(value, -1) {
		start, end := loc[0], loc[1]

		if start > 0 && isNameByte(value[start-1]) {
			continue
		}

		if end < len(value) && isNameByte(value[end]) {
			continue
		}

		return start, end, true
	}

	return 0, 0, false
}

// isNameByte reports whether b can be part of a longer file name.
func isNameByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_':
		return true
	default:
		return false
	}
}

// loadNoConfigAllowlist parses no_config_file_allowlist.txt in dir: one
// entry per line, either a bare repo-relative path or path:line (1-based).
// Blank lines and # comments are ignored. A missing file yields an empty
// allowlist: the rule stays in force, it only has no exception.
func loadNoConfigAllowlist(dir string) (allowlist, error) {
	f, closeFile, err := openContained(dir, "no_config_file_allowlist.txt")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return allowlist{}, nil
		}

		return nil, fmt.Errorf("tagcheck: open no-config-file allowlist: %w", err)
	}
	defer closeFile()

	a := allowlist{}

	parseErr := eachDataLine(f, "no_config_file_allowlist.txt", func(line string) error {
		a[strings.TrimSpace(line)] = true
		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return a, nil
}

// checkLegacyConfigNames reports every place where the file names a retired
// project config file: a string literal (a path, a message, a constant), a
// constant string expression that builds such a name, or a comment. One
// violation is reported per line: the line names the first retired file the
// rule found on it.
//
// The rule covers the file name forms, the format names and the shapes that
// build a name at runtime, because the break means the framework does not
// know these names at all: a stale file is ignored silently, so any mention
// of it in the source is a leftover of the removed formats.
func checkLegacyConfigNames(rel string, fset *token.FileSet, f *ast.File, r rules) []violation {
	var vs []violation

	rule := r.legacy
	if rule == nil {
		return vs
	}

	if r.noConfigAllowed[rel] {
		return vs // the whole file is allowlisted
	}

	reported := map[int]bool{}

	report := func(line int, name string) {
		if reported[line] {
			return
		}

		if r.noConfigAllowed[fmt.Sprintf("%s:%d", rel, line)] {
			return
		}

		reported[line] = true

		vs = append(vs, violation{
			path: rel,
			line: line,
			rule: "no-config-file",
			msg:  fmt.Sprintf("names the retired config file %q; v0.8.0 is env-only and the retired formats (%s) are gone", name, strings.Join(rule.names, ", ")),
		})
	}

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				return true
			}

			value, err := strconv.Unquote(node.Value)
			if err != nil {
				return true
			}

			if name, ok := rule.match(value); ok {
				report(literalLine(fset, rule, node), name)
			}

			return true
		case *ast.BinaryExpr:
			value, ok := constString(node)
			if !ok {
				return true
			}

			if name, ok := rule.match(value); ok {
				report(matchLine(fset, rule, node), name)
				return false
			}

			return true
		case *ast.CallExpr:
			value, ok := constString(node)
			if !ok {
				return true
			}

			if name, ok := rule.match(value); ok {
				report(matchLine(fset, rule, node), name)
				return false
			}

			return true
		}

		return true
	})

	for _, group := range f.Comments {
		for _, comment := range group.List {
			if name, ok := rule.match(comment.Text); ok {
				report(fset.Position(comment.Pos()).Line, name)
			}
		}
	}

	return vs
}

// literalLine returns the line of a string literal that names a retired
// config file: the line the literal starts on, plus the lines the name sits
// below it in a multi-line literal (a raw-string usage text, for example).
func literalLine(fset *token.FileSet, rule *legacyConfigRule, lit *ast.BasicLit) int {
	line := fset.Position(lit.Pos()).Line

	start, _, ok := rule.matchOffset(lit.Value)
	if !ok {
		return line
	}

	return line + strings.Count(lit.Value[:start], "\n")
}

// matchLine returns the line that carries the retired config file name of
// an expression: the line of the literal that contributes the name, or the
// line of the expression itself when the name is built from several parts.
func matchLine(fset *token.FileSet, rule *legacyConfigRule, expr ast.Expr) int {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return matchLine(fset, rule, e.X)
	case *ast.BasicLit:
		return literalLine(fset, rule, e)
	case *ast.BinaryExpr:
		if value, ok := constString(e.X); ok {
			if _, hit := rule.match(value); hit {
				return matchLine(fset, rule, e.X)
			}
		}

		if value, ok := constString(e.Y); ok {
			if _, hit := rule.match(value); hit {
				return matchLine(fset, rule, e.Y)
			}
		}
	case *ast.CallExpr:
		if len(e.Args) > 0 {
			return matchLine(fset, rule, e.Args[0])
		}
	}

	return fset.Position(expr.Pos()).Line
}

// constString evaluates expr to its constant string value, when expr is a
// string literal, a concatenation of constant strings, or a call that
// builds a constant string from literal arguments. The second result reports
// whether the expression is constant at all.
func constString(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}

		value, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}

		return value, true
	case *ast.ParenExpr:
		return constString(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}

		left, ok := constString(e.X)
		if !ok {
			return "", false
		}

		right, ok := constString(e.Y)
		if !ok {
			return "", false
		}

		return left + right, true
	case *ast.CallExpr:
		return constCall(e)
	}

	return "", false
}

// constCall evaluates the calls that build a constant string out of literal
// arguments: fmt.Sprintf with a literal format, and strings.Join with a
// literal slice and a literal separator. A call whose arguments are not
// literals (a runtime value) is not constant and is not evaluated.
func constCall(call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}

	switch pkg.Name + "." + sel.Sel.Name {
	case "fmt.Sprintf":
		if len(call.Args) == 0 {
			return "", false
		}

		format, ok := constString(call.Args[0])
		if !ok {
			return "", false
		}

		args := make([]any, 0, len(call.Args)-1)

		for _, arg := range call.Args[1:] {
			value, ok := constString(arg)
			if !ok {
				return "", false
			}

			args = append(args, value)
		}

		return fmt.Sprintf(format, args...), true
	case "strings.Join":
		if len(call.Args) != 2 {
			return "", false
		}

		sep, ok := constString(call.Args[1])
		if !ok {
			return "", false
		}

		lit, ok := call.Args[0].(*ast.CompositeLit)
		if !ok {
			return "", false
		}

		parts := make([]string, 0, len(lit.Elts))

		for _, elt := range lit.Elts {
			value, ok := constString(elt)
			if !ok {
				return "", false
			}

			parts = append(parts, value)
		}

		return strings.Join(parts, sep), true
	}

	return "", false
}
