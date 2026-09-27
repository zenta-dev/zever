package main

import (
	"fmt"
	"io"

	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
)

// GraphConfig carries every input runGraphWith needs. Screen agents build it
// from huh forms; the flag shell (runGraph) builds it from argv. A nil Out
// defaults to os.Stdout.
type GraphConfig struct {
	Files []string
	Out   io.Writer
}

// rootModuleLabel is the display name used for the implicit (unnamed) module
// a schema with zero `module` blocks resolves into.
const rootModuleLabel = "root"

// runGraph compiles the given .zen files with zero backends (schema
// resolution only) and prints the resolved schema as Mermaid diagrams:
// one entity-relation graph per module, then one module-to-module graph
// whose edges are cross-module RPC returns.
//
// Mermaid (rather than DOT) because it is plain text that renders natively in
// GitHub and most markdown viewers, so the output needs no extra tooling to
// look at — matching the stdout-line convention the other subcommands use.
//
// The graph is emitted best-effort even when compilation reported errors (the
// resolver always returns a partially-resolved schema), but a compile error
// still makes the command exit non-zero, like `zever routes`.
func runGraph(args []string) error {
	paths, err := resolveInputFiles(args)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		reportAutoDiscovery(paths)
	}

	return runGraphWith(GraphConfig{Files: paths})
}

// runGraphWith resolves cfg.Files with zero backends (one shared resolved
// schema) and prints one fenced Mermaid entity-relation block per module
// plus one module-to-module block to cfg.Out. Diagrams print best-effort
// even on compile errors, which still yield a non-nil return.
func runGraphWith(cfg GraphConfig) error {
	out := outOrStdout(cfg.Out)

	files, err := loadFiles(cfg.Files)
	if err != nil {
		return err
	}

	result, diags := compile.Compile(files)

	if len(diags) > 0 {
		printDiagnostics(diags)
	}

	for _, m := range result.Schema.Modules {
		printEntityGraph(out, m)
	}

	printModuleGraph(out, result.Schema)

	if diags.HasErrors() {
		return fmt.Errorf("zever graph: %d file(s) failed to compile", len(files))
	}

	return nil
}

// printEntityGraph emits one fenced Mermaid `graph TD` block for a single
// module: a node per entity, an edge per relation labeled with its kind.
func printEntityGraph(out io.Writer, m *ir.Module) {
	_, _ = fmt.Fprintln(out, "```mermaid")
	_, _ = fmt.Fprintln(out, "graph TD")
	_, _ = fmt.Fprintf(out, "  %%%% module: %s\n", moduleLabel(m))

	for _, e := range m.Entities {
		_, _ = fmt.Fprintf(out, "  %s[\"%s\"]\n", e.Name, e.Name)
	}

	for _, e := range m.Entities {
		for _, r := range e.Relations {
			if r.Target == nil {
				continue
			}

			_, _ = fmt.Fprintf(out, "  %s -->|%s| %s\n", e.Name, relationKindName(r.Kind), r.Target.Name)
		}
	}

	_, _ = fmt.Fprintln(out, "```")
}

// printModuleGraph emits one fenced Mermaid `graph LR` block: a node per
// module, and an edge per distinct cross-module RPC return (a service in
// module A returning an entity owned by module B).
func printModuleGraph(out io.Writer, schema *ir.Schema) {
	_, _ = fmt.Fprintln(out, "```mermaid")
	_, _ = fmt.Fprintln(out, "graph LR")

	for _, m := range schema.Modules {
		label := moduleLabel(m)
		_, _ = fmt.Fprintf(out, "  %s[\"%s\"]\n", label, label)
	}

	seen := make(map[string]bool)

	for _, m := range schema.Modules {
		for _, svc := range m.Services {
			for _, rpc := range svc.Operations {
				if rpc.Returns == nil || rpc.Returns.Module() == nil || rpc.Returns.Module() == m {
					continue
				}

				edge := fmt.Sprintf("  %s -->|rpc| %s\n", moduleLabel(m), moduleLabel(rpc.Returns.Module()))
				if seen[edge] {
					continue
				}

				seen[edge] = true

				_, _ = fmt.Fprint(out, edge)
			}
		}
	}

	_, _ = fmt.Fprintln(out, "```")
}

// moduleLabel returns the Mermaid node id/label for m, mapping the implicit
// unnamed module to "root" (Mermaid node ids cannot be empty).
func moduleLabel(m *ir.Module) string {
	if m == nil || m.Name == "" {
		return rootModuleLabel
	}

	return m.Name
}

// relationKindName renders an ir.RelationKind as its DSL keyword (ir has no
// String() method on the kind — it is a pure data package).
func relationKindName(k ir.RelationKind) string {
	switch k {
	case ir.HasMany:
		return "has_many"
	case ir.HasOne:
		return "has_one"
	case ir.BelongsTo:
		return "belongs_to"
	case ir.ManyToMany:
		return "many_to_many"
	default:
		return "unknown"
	}
}
