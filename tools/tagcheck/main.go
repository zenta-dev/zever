// Command tagcheck scans the repository for struct-tag wire-format
// violations.
//
// Rules:
//   - empty-tag: a struct field whose tag is present but empty (```` or
//     whitespace). Empty tags make encoding/json fall back to Go field
//     names, silently breaking wire formats (the v0.8.0 env-only config
//     migration stripped 169 json tags this way). Non-test, non-generated
//     files only.
//   - forbidden-tag: a yaml:/toml: struct tag outside the allowlist
//     (tag_allowlist.txt). The v0.8.0 migration removed yaml/toml
//     serialization deliberately; reintroducing it fails the gate unless
//     the file is allowlisted with a justification.
//   - golden-json: an exported field of a v0.7.2 wire struct (see
//     golden_wire_structs.txt) lacking a json tag, or a golden struct that
//     no longer exists in the tree. The list is the registry of structs
//     that carried json tags at v0.7.2, the last release before the
//     strip; new structs are not pinned, but existing wire structs must
//     keep their tags.
//
// Skipped: _test.go files, generated files (a "DO NOT EDIT" marker in a
// leading comment), and the .git/.worktrees/.claude/vendor/node_modules/
// generated directories.
//
// Usage: tagcheck [root] (default "."). Prints one "path:line: rule: message"
// line per violation to stdout and exits 1 if any are found, 2 on scan
// error.
package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// violation is one struct-tag rule breach: file, line, rule, detail.
type violation struct {
	path string
	line int
	rule string
	msg  string
}

// goldenStructs is the v0.7.2 wire-struct registry: package dir (relative
// to the scan root) -> struct name -> v0.7.2 source file (documentation).
type goldenStructs map[string]map[string]string

// allowlist holds permitted yaml:/toml: tag locations: bare paths allow the
// whole file, path:line allows one field line (1-based).
type allowlist map[string]bool

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the scan over the root selected by args (default "."),
// loading the golden list and tag allowlist from the tagcheck tool
// directory. The return value is the process exit code: 0 when clean, 1 on
// violations, 2 on scan error.
func run(args []string, stdout, stderr io.Writer) int {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	toolDir := filepath.Join(root, "tools", "tagcheck")

	golden, err := loadGolden(filepath.Join(toolDir, "golden_wire_structs.txt"))
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load golden list: %v\n", err)
		return 2
	}

	allowed, err := loadAllowlist(filepath.Join(toolDir, "tag_allowlist.txt"))
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load tag allowlist: %v\n", err)
		return 2
	}

	vs, err := scan(root, golden, allowed)
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: %v\n", err)
		return 2
	}

	sort.Slice(vs, func(i, j int) bool {
		if vs[i].path != vs[j].path {
			return vs[i].path < vs[j].path
		}

		if vs[i].line != vs[j].line {
			return vs[i].line < vs[j].line
		}

		if vs[i].rule != vs[j].rule {
			return vs[i].rule < vs[j].rule
		}

		return vs[i].msg < vs[j].msg
	})

	for _, v := range vs {
		fmt.Fprintf(stdout, "%s:%d: %s: %s\n", v.path, v.line, v.rule, v.msg)
	}

	if len(vs) > 0 {
		fmt.Fprintf(stderr, "tagcheck: %d violation(s)\n", len(vs))
		return 1
	}

	return 0
}

// loadGolden parses golden_wire_structs.txt: tab-separated package dir,
// struct name, v0.7.2 source file. Blank lines and # comments are
// ignored.
func loadGolden(path string) (goldenStructs, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open golden list: %w", err)
	}
	defer func() { _ = f.Close() }()

	g := goldenStructs{}

	parseErr := eachDataLine(path, func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("tagcheck: malformed golden list line %q", line)
		}

		names := g[parts[0]]
		if names == nil {
			names = map[string]string{}
			g[parts[0]] = names
		}

		if len(parts) >= 3 {
			names[parts[1]] = parts[2]
		} else {
			names[parts[1]] = ""
		}

		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return g, nil
}

// loadAllowlist parses tag_allowlist.txt: one entry per line, either a
// bare repo-relative path or path:line (1-based field line). Blank lines
// and # comments are ignored.
func loadAllowlist(path string) (allowlist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open tag allowlist: %w", err)
	}
	defer func() { _ = f.Close() }()

	a := allowlist{}

	parseErr := eachDataLine(path, func(line string) error {
		a[strings.TrimSpace(line)] = true
		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return a, nil
}

// eachDataLine scans path, invoking fn on every non-blank, non-comment
// line; a non-nil fn error aborts the scan.
func eachDataLine(path string, fn func(string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	for line := 0; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		if err := fn(text); err != nil {
			return fmt.Errorf("%s:%d: %w", path, line+1, err)
		}
	}

	return sc.Err()
}

// scan walks root, checking every non-test, non-generated Go file against
// the three tag rules. seen records golden (dir, struct) pairs found in
// the tree so checkGoldenPresent can report vanished golden structs.
func scan(root string, golden goldenStructs, allowed allowlist) ([]violation, error) {
	var vs []violation

	seen := map[string]bool{}

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

		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		fileVs, isGenerated, scanErr := checkFile(rel, path, golden, allowed)
		if scanErr != nil {
			return scanErr
		}

		vs = append(vs, fileVs...)

		if isGenerated {
			return nil
		}

		dir := filepath.ToSlash(filepath.Dir(rel))
		collectSeen(dir, path, seen)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return append(vs, checkGoldenPresent(golden, seen)...), nil
}

// checkGoldenPresent reports golden structs missing from the tree: a
// removal or move must update the golden list with a justification comment.
func checkGoldenPresent(golden goldenStructs, seen map[string]bool) []violation {
	var vs []violation

	dirs := make([]string, 0, len(golden))
	for dir := range golden {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	for _, dir := range dirs {
		names := golden[dir]

		sortedNames := make([]string, 0, len(names))
		for name := range names {
			sortedNames = append(sortedNames, name)
		}

		sort.Strings(sortedNames)

		for _, name := range sortedNames {
			if seen[dir+"\t"+name] {
				continue
			}

			detail := name
			if origin := names[name]; origin != "" {
				detail = fmt.Sprintf("%s (v0.7.2: %s)", name, origin)
			}

			vs = append(vs, violation{
				path: dir + "/MISSING",
				line: 0,
				rule: "golden-removed",
				msg:  fmt.Sprintf("golden wire struct %s not found in %s; delete its golden entry with a justification comment if the removal was intentional", detail, dir),
			})
		}
	}

	return vs
}

// collectSeen records which golden (dir, struct) pairs the file defines.
func collectSeen(dir, path string, seen map[string]bool) {
	fset := token.NewFileSet()

	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return
	}

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if _, ok := ts.Type.(*ast.StructType); ok {
				seen[dir+"\t"+ts.Name.Name] = true
			}
		}
	}
}

// checkFile parses one Go file and returns its tag-rule violations. The
// second result reports whether the file is generated (DO NOT EDIT marker
// in a leading comment), in which case it is skipped entirely.
func checkFile(rel, path string, golden goldenStructs, allowed allowlist) (vs []violation, generated bool, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}

	for _, cg := range f.Comments {
		if fset.Position(cg.Pos()).Line > 10 {
			break
		}

		for _, c := range cg.List {
			if strings.Contains(c.Text, "DO NOT EDIT") {
				return nil, true, nil
			}
		}
	}

	dir := filepath.ToSlash(filepath.Dir(rel))

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}

		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}

			for _, field := range st.Fields.List {
				if len(field.Names) == 0 {
					continue // embedded fields carry no json key
				}

				isWire := false
				if _, ok := golden[dir][ts.Name.Name]; ok {
					isWire = true
				}

				line := fset.Position(field.Pos()).Line

				var tag reflect.StructTag
				if field.Tag != nil {
					tag = reflect.StructTag(strings.Trim(field.Tag.Value, "`"))

					if strings.TrimSpace(string(tag)) == "" {
						vs = append(vs, violation{
							path: rel,
							line: line,
							rule: "empty-tag",
							msg:  fmt.Sprintf("field %s has an empty struct tag; wire keys fall back to the Go field name", field.Names[0].Name),
						})

						continue
					}

					for _, kind := range []string{"yaml", "toml"} {
						if _, present := tag.Lookup(kind); present && !allowed[rel] && !allowed[fmt.Sprintf("%s:%d", rel, line)] {
							vs = append(vs, violation{
								path: rel,
								line: line,
								rule: "forbidden-tag",
								msg:  fmt.Sprintf("field %s carries a %s: tag outside the tag allowlist (tag_allowlist.txt)", field.Names[0].Name, kind),
							})
						}
					}
				}

				if !isWire {
					continue
				}

				for _, name := range field.Names {
					if !name.IsExported() {
						continue
					}

					if _, present := tag.Lookup("json"); !present {
						vs = append(vs, violation{
							path: rel,
							line: line,
							rule: "golden-json",
							msg:  fmt.Sprintf("exported field %s of wire struct %s lacks a json tag", name.Name, ts.Name.Name),
						})
					}
				}
			}
		}
	}

	return vs, false, nil
}

// skipDir reports whether the walk must not descend into name.
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
