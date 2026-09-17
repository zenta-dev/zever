package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// entityUsage is prepended to `zever generate entity -h`.
const entityUsage = `zever generate entity <module> <name> [--field name:type]...

Appends an entity declaration to internal/<module>/<module>.zen, the file
` + "`zever generate module <module>`" + ` created. The entity always gets an
` + "`id: uuid @primary`" + ` and a ` + "`created_at: timestamp @default(now())`" + `;
every --field is inserted between them, in the order given.

--field may be repeated, and may appear before or after the positional
arguments.

Flags:`

// scalarTypeNames mirrors resolver.scalarTypes: the fixed v1 set of field
// types. It is duplicated here (rather than exported from the resolver) only
// to turn a typo into an immediate CLI error instead of a confusing
// resolve-time diagnostic on the next `zever compile`.
var scalarTypeNames = []string{
	"bool", "bytes", "date", "float32", "float64",
	"int32", "int64", "json", "string", "timestamp", "uuid",
}

// fieldSpecs collects a repeatable --field flag.
type fieldSpecs []fieldSpec

type fieldSpec struct {
	Name string
	Type string
}

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seam keeps behavior identical while enabling success/error coverage.
var promptInputForEntity = promptInput

func (f *fieldSpecs) String() string {
	parts := make([]string, 0, len(*f))
	for _, spec := range *f {
		parts = append(parts, spec.Name+":"+spec.Type)
	}

	return strings.Join(parts, ",")
}

func (f *fieldSpecs) Set(value string) error {
	name, typ, ok := strings.Cut(value, ":")

	name = strings.TrimSpace(name)
	typ = strings.TrimSpace(typ)

	if !ok || name == "" || typ == "" {
		return fmt.Errorf("--field %q: want name:type (for example title:string)", value)
	}

	if !isIdent(name) {
		return fmt.Errorf("--field %q: %q is not a valid field name", value, name)
	}

	if !knownScalar(typ) {
		return fmt.Errorf("--field %q: unknown type %q (want one of %s)", value, typ, strings.Join(scalarTypeNames, ", "))
	}

	*f = append(*f, fieldSpec{Name: name, Type: typ})

	return nil
}

func knownScalar(typ string) bool {
	i := sort.SearchStrings(scalarTypeNames, typ)

	return i < len(scalarTypeNames) && scalarTypeNames[i] == typ
}

// isIdent reports whether s is a legal .zen identifier: an ASCII letter or
// underscore followed by letters, digits or underscores.
func isIdent(s string) bool {
	if s == "" {
		return false
	}

	for i, r := range s {
		switch {
		case r == '_',
			r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
			if i == 0 {
				return false
			}
		default:
			return false
		}
	}

	return true
}

// EntityField is one --field declaration for GenerateEntity: a .zen field
// name and scalar type. It mirrors fieldSpec in exported form so screen
// agents can construct entity input without touching flag parsing.
type EntityField struct {
	Name string
	Type string
}

// GenerateEntityConfig is the pure input to GenerateEntity: the target
// module, the new entity name, and its fields in declaration order.
type GenerateEntityConfig struct {
	Module string
	Name   string
	Fields []EntityField
	Stdout io.Writer
	Stderr io.Writer
}

// GenerateEntity appends the rendered entity declaration to the module's
// .zen file and returns the file path. It performs no flag parsing.
func GenerateEntity(cfg GenerateEntityConfig) (string, error) {
	const tag = "zever generate entity"

	if isTraversalName(cfg.Module) || isTraversalName(cfg.Name) {
		bad := cfg.Module
		if isTraversalName(cfg.Name) {
			bad = cfg.Name
		}

		return "", fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, bad)
	}

	if !isIdent(cfg.Module) || !isIdent(cfg.Name) {
		return "", fmt.Errorf("%s: module and entity name must be identifiers, got %q and %q", tag, cfg.Module, cfg.Name)
	}

	fields := make(fieldSpecs, 0, len(cfg.Fields))
	for _, f := range cfg.Fields {
		if err := fields.Set(f.Name + ":" + f.Type); err != nil {
			return "", fmt.Errorf("%s: %w", tag, err)
		}
	}

	path, _, decl, err := readZenModule(tag, cfg.Module)
	if err != nil {
		return "", err
	}

	if kind, taken := declNameTaken(decl, cfg.Name); taken {
		return "", fmt.Errorf("%s: module %q already declares a %s named %q", tag, cfg.Module, kind, cfg.Name)
	}

	if err := appendDeclBeforeClosingBrace(path, cfg.Module, renderEntityDecl(cfg.Name, fields)); err != nil {
		return "", err
	}

	if stdout := cfg.Stdout; stdout != nil {
		_, _ = fmt.Fprintln(stdout, success("✔ appended ")+bold(fmt.Sprintf("entity %q", cfg.Name))+dim(" to ")+cyan(path))
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint(hintFor("generate entity")))
	}

	return path, nil
}

func runGenerateEntity(args []string) error {
	const tag = "zever generate entity"

	args = peelInteractive(args)

	var fields fieldSpecs

	fs := flag.NewFlagSet("generate entity", flag.ContinueOnError)
	fs.Var(&fields, "field", "a field to declare, as name:type; repeatable")
	interactive := fs.Bool("interactive", false, "prompt for missing values")
	interactiveShort := fs.Bool("i", false, "prompt for missing values (shorthand)")

	fs.Usage = func() {
		_, _ = fmt.Fprintln(fs.Output(), entityUsage)
		fs.PrintDefaults()
	}

	positional, rest := splitPositionals(args, 2)

	if err := fs.Parse(rest); err != nil {
		return err
	}

	positional = append(positional, fs.Args()...)

	if *interactive || *interactiveShort {
		interactiveMode = true
	}

	if len(positional) != 2 {
		if isInteractiveTerminal() {
			if len(positional) < 1 {
				m, err := promptInputForEntity("Module name", "", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				})
				if err != nil {
					return err
				}

				positional = append(positional, m)
			}

			if len(positional) < 2 {
				n, err := promptInputForEntity("Entity name", "", func(s string) error {
					if !isIdent(s) {
						return errors.New("must be identifier")
					}

					return nil
				})
				if err != nil {
					return err
				}

				positional = append(positional, n)
			}
		}

		if len(positional) != 2 {
			return errors.New(tag + ": usage: zever generate entity <module> <name> [--field name:type] (--field is repeatable)")
		}
	}

	module, name := positional[0], positional[1]

	exported := make([]EntityField, 0, len(fields))
	for _, f := range fields {
		exported = append(exported, EntityField(f))
	}

	_, err := GenerateEntity(GenerateEntityConfig{
		Module: module,
		Name:   name,
		Fields: exported,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})

	return err
}

// renderEntityDecl builds the .zen text of a new entity, unindented — the
// splice indents it to its position in the module block.
func renderEntityDecl(name string, fields fieldSpecs) string {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "entity %s {\n", name)
	b.WriteString("\tid: uuid @primary\n")

	for _, f := range fields {
		_, _ = fmt.Fprintf(&b, "\t%s: %s\n", f.Name, f.Type)
	}

	b.WriteString("\tcreated_at: timestamp @default(now())\n")
	b.WriteString("}\n")

	return b.String()
}
