// Command errscan scans the repository for error-convention violations.
//
// Rules:
//   - unprefixed: errors.New/fmt.Errorf literal whose first token has no
//     ":"; messages must look like "<package>: <message>". Skipped in
//     shared/apperror, _test.go files, and anywhere an "// errscan:allow"
//     comment appears (file- or line-level).
//   - bracket-prefix: errors.New/fmt.Errorf literal starting with "[...]"
//     (outside shared/apperror, which emits "[CODE]" messages).
//   - double-%w: two or more "%w" verbs in a single fmt.Errorf format
//     string; use errors.Join to combine a sentinel with a cause.
//   - panic: panic(...) outside dsl/backend/gogen/render_validate.go and
//     _test.go files.
//
// Usage: errscan [root] (default "."). Prints one "path:line: rule: message"
// line per violation and exits 1 if any are found.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	allowMarker = "errscan:allow"
	apperrorPkg = "shared/apperror"
	panicExempt = "dsl/backend/gogen/render_validate.go"
)

var bracketPrefix = regexp.MustCompile(`^\[[^\]\n]*\]`)

type violation struct {
	path string
	line int
	rule string
	msg  string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the scan over the root selected by args (default "."),
// writing violation lines to stdout and the summary or scan error to
// stderr. The return value is the process exit code: 0 when clean, 1 on
// violations, 2 on scan error.
func run(args []string, stdout, stderr io.Writer) int {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	vs, err := scan(root)
	if err != nil {
		fmt.Fprintf(stderr, "errscan: %v\n", err)
		return 2
	}
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].path != vs[j].path {
			return vs[i].path < vs[j].path
		}
		if vs[i].line != vs[j].line {
			return vs[i].line < vs[j].line
		}
		return vs[i].rule < vs[j].rule
	})
	for _, v := range vs {
		fmt.Fprintf(stdout, "%s:%d: %s: %s\n", v.path, v.line, v.rule, v.msg)
	}
	if len(vs) > 0 {
		fmt.Fprintf(stderr, "errscan: %d violation(s)\n", len(vs))
		return 1
	}
	return 0
}

func scan(root string) ([]violation, error) {
	var vs []violation
	// #nosec G703 -- dev-only guard; root is an explicit caller-supplied scan path.
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		fileVs, err := checkFile(rel, path)
		if err != nil {
			return err
		}
		vs = append(vs, fileVs...)
		return nil
	})
	return vs, err
}

func skipDir(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "vendor", "node_modules", "generated":
		return true
	}
	return false
}

func checkFile(rel, path string) ([]violation, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	isTest := strings.HasSuffix(rel, "_test.go")
	isApperror := strings.HasPrefix(rel, apperrorPkg+"/")

	lineStart := []int{0}
	for i, b := range src {
		if b == '\n' {
			lineStart = append(lineStart, i+1)
		}
	}
	standalone := func(c *ast.Comment) bool {
		pos := fset.Position(c.Slash)
		if pos.Line-1 >= len(lineStart) {
			return true
		}
		end := lineStart[pos.Line-1] + pos.Column - 1
		if end > len(src) {
			end = len(src)
		}
		return strings.TrimSpace(string(src[lineStart[pos.Line-1]:end])) == ""
	}

	fileAllow := false
	allowLines := map[int]bool{}
	for _, cg := range file.Comments {
		has := false
		for _, c := range cg.List {
			if strings.Contains(c.Text, allowMarker) {
				has = true
				break
			}
		}
		if !has {
			continue
		}
		for _, c := range cg.List {
			if !strings.Contains(c.Text, allowMarker) {
				continue
			}
			if standalone(c) {
				fileAllow = true
			} else {
				allowLines[fset.Position(c.Slash).Line] = true
			}
		}
	}

	var vs []violation
	add := func(rule string, line int, format string, args ...any) {
		if fileAllow || allowLines[line] {
			return
		}
		vs = append(vs, violation{path: rel, line: line, rule: rule, msg: fmt.Sprintf(format, args...)})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		line := fset.Position(call.Pos()).Line
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == "panic" {
				if !isTest && rel != panicExempt {
					add("panic", line, "panic outside %s (resolver-invariant guards only)", panicExempt)
				}
				return true
			}
			if fun.Name == "New" || fun.Name == "Errorf" {
				checkCall(call, fun.Name == "Errorf", isTest, isApperror, add, line)
			}
		case *ast.SelectorExpr:
			if ident, ok := fun.X.(*ast.Ident); ok {
				if (ident.Name == "errors" && fun.Sel.Name == "New") ||
					(ident.Name == "fmt" && fun.Sel.Name == "Errorf") {
					checkCall(call, fun.Sel.Name == "Errorf", isTest, isApperror, add, line)
				}
			}
		}
		return true
	})
	return vs, nil
}

func checkCall(call *ast.CallExpr, isErrorf, isTest, isApperror bool, add func(string, int, string, ...any), line int) {
	if len(call.Args) == 0 {
		return
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return
	}
	val, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	if !isTest && !isApperror && !strings.Contains(val, ":") {
		add("unprefixed", line, "error message %q has no \"<package>: \" prefix", val)
	}
	if !isApperror && bracketPrefix.MatchString(val) {
		add("bracket-prefix", line, "error message %q uses a bracket prefix; want \"<package>: <message>\"", val)
	}
	if isErrorf && strings.Count(val, "%w") >= 2 {
		add("double-%w", line, "fmt.Errorf %q wraps multiple errors; use errors.Join", val)
	}
}
