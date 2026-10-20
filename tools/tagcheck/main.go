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
//   - golden-json: an exported field of a registered wire struct (see
//     golden_wire_structs.txt) lacking a json tag, or a golden struct that
//     no longer exists in the tree. The list is the registry of structs
//     that carried json tags at v0.7.2, the last release before the
//     strip; new structs are not pinned, but existing wire structs must
//     keep their tags.
//   - golden-key: an exported field of a registered wire struct whose json
//     key differs from the pinned one (golden_wire_fields.txt), whose json
//     key is empty (the wire falls back to the Go field name), or that is
//     dropped from the wire (json:"-"). A renamed key is a silent wire
//     break: the field still marshals, under a key no consumer reads.
//   - golden-missing: a pinned wire key the struct no longer exposes, for
//     example after the field was deleted or renamed together with its
//     key.
//   - golden-nofields: a registered wire struct with no pinned keys: the
//     golden field data was not regenerated after the struct was
//     registered.
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
	"slices"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/tools/tagcheck/internal/wire"
)

// violation is one struct-tag rule breach: file, line, rule, detail.
type violation struct {
	path string
	line int
	rule string
	msg  string
}

// goldenStructs is the wire-struct registry: package dir (relative to the
// scan root) -> struct name -> v0.7.2 source file (documentation).
type goldenStructs map[string]map[string]string

// goldenFields pins the wire key of every field of a registered wire
// struct: package dir -> struct name -> Go field name -> wire key.
type goldenFields map[string]map[string]map[string]string

// exceptions lists the structs that expose json keys but are deliberately
// not registered as wire structs: package dir -> struct name ->
// justification.
type exceptions map[string]map[string]string

// allowlist holds permitted yaml:/toml: tag locations: bare paths allow the
// whole file, path:line allows one field line (1-based).
type allowlist map[string]bool

// rules holds the golden data the scan compares the tree against.
type rules struct {
	structs goldenStructs
	fields  goldenFields
	except  exceptions
	allowed allowlist
}

// structInfo summarizes one struct type found in the tree.
type structInfo struct {
	line     int
	exported int
	tagged   int
}

// fieldInfo is one exported struct field with its source line.
type fieldInfo struct {
	wire.Field
	line int
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the scan over the root selected by args (default "."),
// loading the golden lists and tag allowlist from the tagcheck tool
// directory. The return value is the process exit code: 0 when clean, 1 on
// violations, 2 on scan error.
func run(args []string, stdout, stderr io.Writer) int {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}

	toolDir := filepath.Join(root, "tools", "tagcheck")

	structs, err := loadGolden(toolDir)
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load golden list: %v\n", err)
		return 2
	}

	fields, err := loadGoldenFields(toolDir)
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load golden field list: %v\n", err)
		return 2
	}

	allowed, err := loadAllowlist(toolDir)
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load tag allowlist: %v\n", err)
		return 2
	}

	except, err := loadExceptions(toolDir)
	if err != nil {
		fmt.Fprintf(stderr, "tagcheck: load golden exception list: %v\n", err)
		return 2
	}

	vs, err := scan(root, rules{structs: structs, fields: fields, except: except, allowed: allowed})
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

// openContained opens name inside dir. The directory is opened as an
// os.Root, so name can never escape it, whatever it contains.
func openContained(dir, name string) (*os.File, func(), error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, func() {}, err
	}

	f, err := root.Open(name)
	if err != nil {
		_ = root.Close()
		return nil, func() {}, err
	}

	return f, func() {
		_ = f.Close()
		_ = root.Close()
	}, nil
}

// loadGolden parses golden_wire_structs.txt in dir: tab-separated package
// dir, struct name, v0.7.2 source file. Blank lines and # comments are
// ignored.
func loadGolden(dir string) (goldenStructs, error) {
	f, closeFile, err := openContained(dir, "golden_wire_structs.txt")
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open golden list: %w", err)
	}
	defer closeFile()

	g := goldenStructs{}

	parseErr := eachDataLine(f, "golden_wire_structs.txt", func(line string) error {
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

// loadGoldenFields parses golden_wire_fields.txt in dir: tab-separated
// package dir, struct name, Go field name, pinned wire key, and the base
// revision the key was taken from. Blank lines and # comments are ignored.
func loadGoldenFields(dir string) (goldenFields, error) {
	f, closeFile, err := openContained(dir, "golden_wire_fields.txt")
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open golden field list: %w", err)
	}
	defer closeFile()

	g := goldenFields{}

	parseErr := eachDataLine(f, "golden_wire_fields.txt", func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 4 || parts[0] == "" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
			return fmt.Errorf("tagcheck: malformed golden field line %q", line)
		}

		names := g[parts[0]]
		if names == nil {
			names = map[string]map[string]string{}
			g[parts[0]] = names
		}

		fields := names[parts[1]]
		if fields == nil {
			fields = map[string]string{}
			names[parts[1]] = fields
		}

		fields[parts[2]] = parts[3]

		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return g, nil
}

// loadExceptions parses golden_wire_exceptions.txt in dir: tab-separated
// package dir, struct name, and the justification for the exception.
// Blank lines and # comments are ignored.
func loadExceptions(dir string) (exceptions, error) {
	f, closeFile, err := openContained(dir, "golden_wire_exceptions.txt")
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open golden exception list: %w", err)
	}
	defer closeFile()

	e := exceptions{}

	parseErr := eachDataLine(f, "golden_wire_exceptions.txt", func(line string) error {
		parts := strings.Split(line, "\t")
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return fmt.Errorf("tagcheck: malformed golden exception line %q", line)
		}

		names := e[parts[0]]
		if names == nil {
			names = map[string]string{}
			e[parts[0]] = names
		}

		names[parts[1]] = parts[2]

		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return e, nil
}

// loadAllowlist parses tag_allowlist.txt in dir: one entry per line,
// either a bare repo-relative path or path:line (1-based field line).
// Blank lines and # comments are ignored.
func loadAllowlist(dir string) (allowlist, error) {
	f, closeFile, err := openContained(dir, "tag_allowlist.txt")
	if err != nil {
		return nil, fmt.Errorf("tagcheck: open tag allowlist: %w", err)
	}
	defer closeFile()

	a := allowlist{}

	parseErr := eachDataLine(f, "tag_allowlist.txt", func(line string) error {
		a[strings.TrimSpace(line)] = true
		return nil
	})
	if parseErr != nil {
		return nil, parseErr
	}

	return a, nil
}

// eachDataLine scans r, invoking fn on every non-blank, non-comment line;
// a non-nil fn error aborts the scan. The origin label is used to report
// the offending line.
func eachDataLine(r io.Reader, origin string, fn func(string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)

	for line := 0; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		if err := fn(text); err != nil {
			return fmt.Errorf("%s:%d: %w", origin, line+1, err)
		}
	}

	return sc.Err()
}

// scan walks root, checking every non-test, non-generated Go file against
// the tag rules. seen records golden (dir, struct) pairs found in the tree
// so checkGoldenPresent can report vanished golden structs.
func scan(root string, r rules) ([]violation, error) {
	var vs []violation

	seen := map[string]bool{}
	found := map[string]map[string]structInfo{}

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

		fileVs, isGenerated, decls, scanErr := checkFile(rel, path, r)
		if scanErr != nil {
			return scanErr
		}

		vs = append(vs, fileVs...)

		if isGenerated {
			return nil
		}

		dir := filepath.ToSlash(filepath.Dir(rel))
		collectSeen(dir, decls, seen, found)

		return nil
	})
	if err != nil {
		return nil, err
	}

	vs = append(vs, checkGoldenPresent(r.structs, seen)...)
	vs = append(vs, checkExceptions(r.except, r.structs, found)...)

	return vs, nil
}

// checkExceptions validates the exception list: every exception must name
// a struct that exists, is not registered (a registered struct is pinned
// already), and is not fully json-tagged (such a struct belongs in the
// registry, where its keys get pinned).
func checkExceptions(except exceptions, structs goldenStructs, found map[string]map[string]structInfo) []violation {
	var vs []violation

	dirs := make([]string, 0, len(except))
	for dir := range except {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	for _, dir := range dirs {
		names := except[dir]

		sortedNames := make([]string, 0, len(names))
		for name := range names {
			sortedNames = append(sortedNames, name)
		}

		sort.Strings(sortedNames)

		for _, name := range sortedNames {
			info, ok := found[dir][name]
			if !ok {
				vs = append(vs, violation{
					path: dir + "/MISSING",
					line: 0,
					rule: "exception-stale",
					msg:  fmt.Sprintf("exception names wire struct %s, which no longer exists in %s; delete the exception", name, dir),
				})

				continue
			}

			if _, registered := structs[dir][name]; registered {
				vs = append(vs, violation{
					path: dir,
					line: info.line,
					rule: "exception-registered",
					msg:  fmt.Sprintf("wire struct %s is registered in golden_wire_structs.txt; drop its exception", name),
				})

				continue
			}

			if info.exported > 0 && info.tagged == info.exported {
				vs = append(vs, violation{
					path: dir,
					line: info.line,
					rule: "exception-unnecessary",
					msg:  fmt.Sprintf("wire struct %s is fully json-tagged; register it in golden_wire_structs.txt instead of excepting it", name),
				})
			}
		}
	}

	return vs
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

// collectSeen records which golden (dir, struct) pairs the file defines,
// and the shape of every struct it declares.
func collectSeen(dir string, decls []structDecl, seen map[string]bool, found map[string]map[string]structInfo) {
	for _, decl := range decls {
		seen[dir+"\t"+decl.name] = true

		byName := found[dir]
		if byName == nil {
			byName = map[string]structInfo{}
			found[dir] = byName
		}

		byName[decl.name] = decl.info
	}
}

// structDecl is one struct type declared in a file.
type structDecl struct {
	name string
	info structInfo
}

// checkFile parses one Go file and returns its tag-rule violations. The
// second result reports whether the file is generated (DO NOT EDIT marker
// in a leading comment), in which case it is skipped entirely. The third
// result lists the struct types the file declares.
func checkFile(rel, path string, r rules) (vs []violation, generated bool, decls []structDecl, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, false, nil, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, false, nil, err
	}

	for _, cg := range f.Comments {
		if fset.Position(cg.Pos()).Line > 10 {
			break
		}

		for _, c := range cg.List {
			if strings.Contains(c.Text, "DO NOT EDIT") {
				return nil, true, nil, nil
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

			fields := collectFields(fset, st)

			info := structInfo{line: fset.Position(st.Pos()).Line, exported: len(fields)}
			for _, field := range fields {
				if field.State() != wire.StateMissing {
					info.tagged++
				}
			}

			decls = append(decls, structDecl{name: ts.Name.Name, info: info})

			vs = append(vs, checkStruct(rel, dir, fset.Position(st.Pos()).Line, ts.Name.Name, fields, r)...)
		}
	}

	return vs, false, decls, nil
}

// collectFields returns the exported fields of st with their source lines.
func collectFields(fset *token.FileSet, st *ast.StructType) []fieldInfo {
	var fields []fieldInfo

	for _, field := range st.Fields.List {
		if len(field.Names) == 0 {
			continue // embedded fields carry no json key
		}

		line := fset.Position(field.Pos()).Line

		for _, ident := range field.Names {
			if !ident.IsExported() {
				continue
			}

			fld := fieldInfo{line: line}
			fld.Name = ident.Name
			if field.Tag != nil {
				fld.Tag = strings.Trim(field.Tag.Value, "`")
				fld.HasTag = true
			}

			fields = append(fields, fld)
		}
	}

	return fields
}

// checkStruct applies the per-struct rules to one struct type.
func checkStruct(rel, dir string, line int, name string, fields []fieldInfo, r rules) []violation {
	var vs []violation

	registered := false
	if names, ok := r.structs[dir]; ok {
		_, registered = names[name]
	}

	pinned := r.fields[dir][name]

	if !registered {
		vs = append(vs, checkUnregistered(rel, dir, line, name, fields, r)...)
	}

	for _, field := range fields {
		if field.HasTag && strings.TrimSpace(field.Tag) == "" {
			vs = append(vs, violation{
				path: rel,
				line: field.line,
				rule: "empty-tag",
				msg:  fmt.Sprintf("field %s has an empty struct tag; wire keys fall back to the Go field name", field.Name),
			})

			continue
		}

		for _, kind := range []string{"yaml", "toml"} {
			if _, present := reflect.StructTag(field.Tag).Lookup(kind); present &&
				!r.allowed[rel] && !r.allowed[fmt.Sprintf("%s:%d", rel, field.line)] {
				vs = append(vs, violation{
					path: rel,
					line: field.line,
					rule: "forbidden-tag",
					msg:  fmt.Sprintf("field %s carries a %s: tag outside the tag allowlist (tag_allowlist.txt)", field.Name, kind),
				})
			}
		}

		if !registered {
			continue
		}

		if field.State() == wire.StateMissing {
			vs = append(vs, violation{
				path: rel,
				line: field.line,
				rule: "golden-json",
				msg:  fmt.Sprintf("exported field %s of wire struct %s lacks a json tag", field.Name, name),
			})

			continue
		}

		want, hasPinned := pinned[field.Name]
		if !hasPinned {
			continue
		}

		if v := checkGoldenKey(rel, field.line, name, field, want); v != nil {
			vs = append(vs, *v)
		}
	}

	vs = append(vs, checkPinnedKeys(rel, line, name, fields, pinned, registered)...)

	return vs
}

// checkUnregistered reports a struct that exposes json keys but is not
// registered as a wire struct. Such a struct reaches a json marshaller
// with keys nobody pinned, so it must be registered (and its keys pinned)
// or excepted with a justification.
func checkUnregistered(rel, dir string, line int, name string, fields []fieldInfo, r rules) []violation {
	vs := make([]violation, 0, 1)

	exposesKey := false

	for _, field := range fields {
		if field.State() == wire.StateNamed {
			exposesKey = true

			break
		}
	}

	if !exposesKey {
		return vs
	}

	if _, excepted := r.except[dir][name]; excepted {
		return vs
	}

	return append(vs, violation{
		path: rel,
		line: line,
		rule: "unregistered-wire",
		msg:  fmt.Sprintf("struct %s exposes json keys but is not registered as a wire struct; add it to golden_wire_structs.txt or justify it in golden_wire_exceptions.txt", name),
	})
}

// checkGoldenKey compares the wire key a field is encoded under with the
// key pinned in the golden field list. It returns nil when they agree.
//
// A pinned key of "-" means the field must stay off the wire.
func checkGoldenKey(rel string, line int, structName string, field fieldInfo, want string) *violation {
	got, onWire := effectiveKey(field)

	switch {
	case want == "-" && !onWire:
		return nil
	case want == "-":
		return &violation{
			path: rel,
			line: line,
			rule: "golden-key",
			msg:  fmt.Sprintf("field %s of wire struct %s is on the wire again (json key %q); the golden contract keeps it off the wire", field.Name, structName, got),
		}
	case !onWire:
		return &violation{
			path: rel,
			line: line,
			rule: "golden-key",
			msg:  fmt.Sprintf("field %s of wire struct %s is dropped from the wire (json:\"-\"); the golden wire key is %q", field.Name, structName, want),
		}
	case got == want:
		return nil
	case field.State() == wire.StateEmpty:
		return &violation{
			path: rel,
			line: line,
			rule: "golden-key",
			msg:  fmt.Sprintf("field %s of wire struct %s has an empty json key; the wire falls back to the Go field name %q, the golden wire key is %q", field.Name, structName, got, want),
		}
	default:
		return &violation{
			path: rel,
			line: line,
			rule: "golden-key",
			msg:  fmt.Sprintf("field %s of wire struct %s exposes the json key %q; the golden wire key is %q", field.Name, structName, got, want),
		}
	}
}

// effectiveKey returns the wire key a field is encoded under and whether
// the field is on the wire at all. encoding/json falls back to the Go
// field name when the json tag carries no name.
func effectiveKey(field fieldInfo) (string, bool) {
	return field.Key()
}

// checkPinnedKeys reports pinned wire keys the struct no longer exposes and
// registered structs whose keys were never pinned.
func checkPinnedKeys(rel string, line int, name string, fields []fieldInfo, pinned map[string]string, registered bool) []violation {
	var vs []violation

	if !registered {
		return vs
	}

	if len(pinned) == 0 {
		if len(fields) == 0 {
			return vs // a struct without exported fields has no wire key to pin
		}

		return append(vs, violation{
			path: rel,
			line: line,
			rule: "golden-nofields",
			msg:  fmt.Sprintf("wire struct %s has no pinned json keys; regenerate golden_wire_fields.txt", name),
		})
	}

	present := map[string]bool{}
	dropped := false

	for _, field := range fields {
		if field.State() == wire.StateDropped {
			dropped = true

			continue
		}

		key, onWire := effectiveKey(field)
		if onWire {
			present[key] = true
		}
	}

	names := make([]string, 0, len(pinned))
	for field := range pinned {
		names = append(names, field)
	}

	sort.Strings(names)

	for _, field := range names {
		key := pinned[field]

		if slices.ContainsFunc(fields, func(f fieldInfo) bool { return f.Name == field }) {
			continue
		}

		if key == "-" {
			if dropped {
				continue
			}

			vs = append(vs, violation{
				path: rel,
				line: line,
				rule: "golden-missing",
				msg:  fmt.Sprintf("wire struct %s no longer drops the field %s from the wire (json:\"-\")", name, field),
			})

			continue
		}

		if present[key] {
			continue // the Go field was renamed, the wire key survived
		}

		vs = append(vs, violation{
			path: rel,
			line: line,
			rule: "golden-missing",
			msg:  fmt.Sprintf("wire struct %s no longer exposes the golden wire key %q", name, key),
		})
	}

	return vs
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
