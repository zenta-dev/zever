package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// skill.go implements `zever docs --skill`: it renders a Claude Code agent
// skill (SKILL.md) for zever from a static prose template plus the live
// command catalog, so the skill never drifts from the CLI it documents. The
// committed snapshot lives at skills/zever/SKILL.md; skill_test.go pins the
// generator output against it.

// renderSkill builds the SKILL.md content for the given command tree.
func renderSkill(root *cobra.Command) string {
	var b strings.Builder

	b.WriteString("---\n")
	b.WriteString("name: zever\n")
	b.WriteString("description: Use when scaffolding, inspecting, or running a zever Go application: scaffold apps and batteries, validate and compile .zen schemas, run servers and workers, or drive the CLI from scripts. Trigger on requests like \"scaffold zever app\", \"add battery\", \"check schema\", \"compile schemas\", or \"run zever\".\n")
	b.WriteString("---\n\n")

	b.WriteString("# zever skill\n\n")
	b.WriteString("Zever is a Go multi-module framework: a schema-driven scaffolding compiler (`.zen` DSL) plus swappable backend batteries. This skill covers the CLI surface an agent needs; library design lives in `AGENTS.md` and `docs/`.\n\n")

	b.WriteString("## Machine interfaces\n\n")
	b.WriteString("Prefer machine output over human output:\n\n")
	b.WriteString("- `zever --json <command>`: JSON envelope `{ok, command, exitCode, data?, error?}` on stdout. Exit codes: 0 success, 1 runtime error, 2 flag misuse.\n")
	b.WriteString("- `zever --help --agent`: machine-readable command catalog (this list, as JSON).\n")
	b.WriteString("- `compile`, `check`, `explain`, `doctor` emit structured `data` under `--json`.\n")
	b.WriteString("- MCP server: build `tools/zever-mcp` and register the binary as an MCP stdio server (`zever_compile`, `zever_schema`, `zever_explain`, `zever_doctor`, `zever_generate`).\n\n")

	b.WriteString("## Workflows\n\n")
	b.WriteString("### Scaffold and run\n\n")
	b.WriteString("```sh\n")
	b.WriteString("zever new myapp --dry-run   # preview first\n")
	b.WriteString("zever new myapp\n")
	b.WriteString("cd myapp && go mod tidy\n")
	b.WriteString("zever check schema/app.zen --json\n")
	b.WriteString("zever compile schema/app.zen --backend=zenorm,proto,atlas\n")
	b.WriteString("zever db migrate --adapter=sqlite --dsn=data/app.db schema/app.zen\n")
	b.WriteString("zever serve\n")
	b.WriteString("```\n\n")

	b.WriteString("### Add a battery\n\n")
	b.WriteString("```sh\n")
	b.WriteString("zever add cache/redis --dry-run   # preview edits\n")
	b.WriteString("zever add cache/redis && go mod tidy\n")
	b.WriteString("```\n\n")

	b.WriteString("### Inspect a schema\n\n")
	b.WriteString("```sh\n")
	b.WriteString("zever routes schema/app.zen\n")
	b.WriteString("zever explain UserService.GetUser schema/app.zen --json\n")
	b.WriteString("zever compile schema/app.zen --backend=mcp --out ./gen\n")
	b.WriteString("```\n\n")

	b.WriteString("## Safety\n\n")
	b.WriteString("- Every mutating command accepts `--dry-run`: preview before writing.\n")
	b.WriteString("- `zever tinker` evaluates arbitrary Go against a live container: development databases only, never production.\n")
	b.WriteString("- Never log secrets or raw option maps; errors name services and fields only.\n\n")

	b.WriteString("## Commands\n\n")

	names := []string{}
	seen := map[string]bool{}

	var walk func(cmds []*cobra.Command)
	walk = func(cmds []*cobra.Command) {
		for _, c := range cmds {
			// Skip the root itself; key by full command path so nested
			// commands (e.g. `zever db migrate`) render correctly and
			// same-named leaves under different parents never collide.
			if c.Name() == "zever" {
				walk(c.Commands())
				continue
			}

			path := c.CommandPath()
			if c.Hidden || seen[path] {
				continue
			}

			seen[path] = true
			short := c.Short
			if short == "" {
				short = "(see -h)"
			}

			names = append(names, "- `"+path+"`: "+short)
			walk(c.Commands())
		}
	}
	walk(root.Commands())
	sort.Strings(names)

	// The `help` command is auto-added by Cobra on Execute, so it is absent
	// from a fresh tree: pin it explicitly for deterministic output.
	if !seen["zever help"] {
		names = append(names, "- `zever help`: Help about any command")
		sort.Strings(names)
	}

	for _, line := range names {
		b.WriteString(line + "\n")
	}

	return b.String()
}

// writeSkillFile writes the rendered skill to <dir>/SKILL.md.
func writeSkillFile(dir string, root *cobra.Command) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("zever docs: create dir: %w", err)
	}

	path := filepath.Join(dir, "SKILL.md")

	if err := os.WriteFile(path, []byte(renderSkill(root)), 0o644); err != nil { //nolint:gosec // generated docs, not a secret
		return fmt.Errorf("zever docs: write skill: %w", err)
	}

	return nil
}
