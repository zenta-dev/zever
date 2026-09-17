package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/parser"
)

// moduleZenPath returns the .zen file `zever generate` targets for module.
// With dir-derived modules, the module identity is the schema subdirectory:
// schema/<module>/<module>.zen (fallback to ProjectConfig.SchemaDir).
func moduleZenPath(module string) string {
	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil && pc.SchemaDir != "" {
		schemaDir = pc.SchemaDir
	}

	return filepath.Join(schemaDir, module, module+".zen")
}

// readZenModule reads and parses the .zen file backing module, returning its
// raw bytes and the parsed *ast.File.
//
// It is the shared precondition check of every append subcommand.
//
//nolint:unparam // src is unused by current callers but part of the tested 4-tuple contract
func readZenModule(tag, module string) (path string, src []byte, file *ast.File, err error) {
	if isTraversalName(module) {
		return "", nil, nil, fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, module)
	}

	candidates := []string{moduleZenPath(module), filepath.Join("internal", module, module+".zen")}
	for _, p := range candidates {
		data, rerr := os.ReadFile(p) //nolint:gosec // path is derived from a developer-supplied module name
		if rerr != nil {
			continue
		}

		if f, diags := parseZen(p, data); !diags.HasErrors() {
			// For new flat files, any file under candidates qualifies if it exists;
			// for legacy module-wrapper tests, also accept internal/ path.
			return p, data, f, nil
		}
		// File exists but has parse errors — still return with error gate below.
		file, diags := parseZen(p, data)
		if diags.HasErrors() {
			printDiagnostics(diags)
			return "", nil, nil, fmt.Errorf("%s: %q does not parse; fix it before generating into it", tag, p)
		}

		return p, data, file, nil
	}

	path = moduleZenPath(module)

	return "", nil, nil, fmt.Errorf("%s: read %q (run `zever generate module %s` first?): %w", tag, path, module, os.ErrNotExist)
}

// parseZen is a thin wrapper over the DSL parser, kept in one place so every
// caller here reads and validates .zen source identically.
func parseZen(path string, src []byte) (*ast.File, diag.List) {
	return parser.New(path, src).ParseFile()
}

// declNameTaken reports whether file already declares something called name,
// regardless of declaration kind. Entities, services, jobs and schedules share
// one namespace in the resolver.
func declNameTaken(file *ast.File, name string) (kind string, taken bool) {
	check := func(d ast.Decl) (string, bool) {
		switch decl := d.(type) {
		case *ast.EntityDecl:
			if decl.Name == name {
				return "entity", true
			}
		case *ast.ServiceDecl:
			if decl.Name == name {
				return "service", true
			}
		case *ast.JobDecl:
			if decl.Name == name {
				return "job", true
			}
		case *ast.ScheduleDecl:
			if decl.Name == name {
				return "schedule", true
			}
		case *ast.MessageDecl:
			if decl.Name == name {
				return "message", true
			}
		}

		return "", false
	}
	for _, d := range file.Decls {
		if k, ok := check(d); ok {
			return k, true
		}
	}

	return "", false
}

// appendDeclBeforeClosingBrace appends decl to the module file at path.
// Module identity is dir-derived (no module { } wrapper in the source), so
// this simply appends decl at EOF.
func appendDeclBeforeClosingBrace(path, _, decl string) error {
	src, err := os.ReadFile(path) //nolint:gosec // path is derived from a developer-supplied module name
	if err != nil {
		return fmt.Errorf("[zever] read %q: %w", path, err)
	}

	if _, diags := parseZen(path, src); diags.HasErrors() {
		printDiagnostics(diags)

		return fmt.Errorf("[zever] %q does not parse; refusing to edit it", path)
	}

	next := appendDecl(src, decl)

	if _, diags := parseZen(path, next); diags.HasErrors() {
		printDiagnostics(diags)

		return fmt.Errorf("[zever] generated declaration would not parse; %q left unchanged", path)
	}

	return writeAtomically(path, next)
}

// appendDecl inserts decl at end of src with proper newline handling.
func appendDecl(src []byte, decl string) []byte {
	trimmed := strings.TrimRight(string(src), " \t\r\n")
	body := strings.TrimRight(decl, "\n")

	var b strings.Builder
	if trimmed == "" {
		b.WriteString(body)
		b.WriteString("\n")

		return []byte(b.String())
	}

	b.WriteString(trimmed)
	b.WriteString("\n\n")
	b.WriteString(body)
	b.WriteString("\n")

	return []byte(b.String())
}

// writeAtomically writes content to a sibling temp file and renames it over
// path, so a failure mid-write can never leave a truncated schema behind.
func writeAtomically(path string, content []byte) error {
	dir, base := filepath.Dir(path), filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".zever-*")
	if err != nil {
		return fmt.Errorf("[zever] create temp file next to %q: %w", path, err)
	}

	tmpPath := tmp.Name()

	defer func() {
		_ = os.Remove(tmpPath) //nolint:gosec // os.CreateTemp's own name, inside path's directory
	}()

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("[zever] write %q: %w", tmpPath, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("[zever] close %q: %w", tmpPath, err)
	}

	if err := os.Chmod(tmpPath, 0o644); err != nil { //nolint:gosec // a schema file, not a secret
		return fmt.Errorf("[zever] chmod %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil { //nolint:gosec // os.CreateTemp's own name, renamed onto the file it was created beside
		return fmt.Errorf("[zever] rename %q -> %q: %w", tmpPath, path, err)
	}

	return nil
}

// indentLines prefixes every non-blank line of s with indent.
func indentLines(s, indent string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}

		lines[i] = indent + line
	}

	return strings.Join(lines, "\n")
}

// byteOffset converts a lexer position (1-based line, 1-based column counted
// in runes) into a byte offset into src.
func byteOffset(src []byte, pos diag.Position) (int, error) {
	line, col := 1, 1

	for i := 0; i < len(src); {
		if line == pos.Line && col == pos.Col {
			return i, nil
		}

		r, size := utf8.DecodeRune(src[i:])
		i += size

		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}

	return 0, fmt.Errorf("[zever] position %s does not address a character in the file", pos)
}
