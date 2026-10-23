package main

// cmd_facts_golden_test.go harvests the zever CLI surface (commands,
// hidden/deprecated metadata, aliases, flags, shorthands, globals) as an
// in-process fact sheet from the Cobra tree built by newRootCmd, and pins it
// against a committed golden (testdata/cmd_facts.json).
//
// Run with -update to rewrite the golden after an intentional CLI-surface
// change, then review the diff before committing. Without -update the test
// diffs the harvest against the golden and fails on drift, so any flag or
// command added, renamed, hidden, deprecated, or aliased (even a quiet or
// hidden one) bites here.
//
// -update is owned here by charmbracelet/x/exp/golden (pulled in with the
// test binary): the same flag name the dsl compile goldens bind on their own
// test flags. Reuse it via Lookup owning nothing when it is missing.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// updateCmdFacts owns -update only when no other package in the test binary
// registered it (the exp/golden init runs first); the dereferencer below
// falls back to looking the flag up so both shapes work.
var updateCmdFacts = func() *bool {
	if flag.CommandLine.Lookup("update") != nil {
		return nil
	}

	b := flag.Bool("update", false, "rewrite testdata/cmd_facts.json from the live harvest")

	return b
}()

// updateCmdFactsRequested reports whether the test binary was run with
// -update, whoever registered the flag.
func updateCmdFactsRequested() bool {
	if updateCmdFacts != nil {
		return *updateCmdFacts
	}

	if f := flag.CommandLine.Lookup("update"); f != nil {
		return f.Value.String() == "true"
	}

	return false
}

// cmdFactsFlag is one harvested flag; every field is pinned so nothing can
// change shape without the golden noticing.
type cmdFactsFlag struct {
	Name       string `json:"name"`
	Shorthand  string `json:"shorthand"`
	Hidden     bool   `json:"hidden"`
	Deprecated string `json:"deprecated"`
}

// cmdFactsCommand is one harvested command: name path, visibility, alias and
// deprecation metadata, plus its local flag facts (persistent flags of the
// root itself are excluded here; they are pinned once as globals).
type cmdFactsCommand struct {
	Path       string         `json:"path"`
	Aliases    []string       `json:"aliases"`
	Hidden     bool           `json:"hidden"`
	Deprecated string         `json:"deprecated"`
	Flags      []cmdFactsFlag `json:"flags"`
}

// cmdFacts is the full CLI fact sheet: global (root persistent) flags plus
// every command in the tree, hidden and legacy spellings included.
type cmdFacts struct {
	Globals  []cmdFactsFlag    `json:"globals"`
	Commands []cmdFactsCommand `json:"commands"`
}

// harvestFlag facts out of one pflag Flag.
func harvestFlag(f *pflag.Flag) cmdFactsFlag {
	return cmdFactsFlag{
		Name:       f.Name,
		Shorthand:  f.Shorthand,
		Hidden:     f.Hidden,
		Deprecated: f.Deprecated,
	}
}

// harvestFlags walks one command's flag sets via VisitAll (so hidden and
// deprecated flags are never skipped) and returns them fully sorted for a
// deterministic golden regardless of SortFlags.
func harvestFlags(cmd *cobra.Command) (local, inherited []cmdFactsFlag) {
	collect := func(set *pflag.FlagSet, into *[]cmdFactsFlag) {
		set.VisitAll(func(f *pflag.Flag) {
			*into = append(*into, harvestFlag(f))
		})
	}

	collect(cmd.LocalFlags(), &local)
	collect(cmd.InheritedFlags(), &inherited)

	sortFlagsFacts(local)
	sortFlagsFacts(inherited)

	return local, inherited
}

// harvestCmdFacts builds the fact sheet from a fresh root command. It walks
// the full tree (hidden commands and aliases included) and initializes the
// auto-injected help and version flags cobra only wires at Execute time, so
// the golden reflects what --help and the parser actually see.
func harvestCmdFacts(root *cobra.Command) cmdFacts {
	facts := cmdFacts{}

	root.InitDefaultHelpFlag()
	root.InitDefaultVersionFlag()

	// The bare root's flags are the globals: persistent (interactive,
	// quiet, no-color, json, V) plus the cobra-injected help and version
	// local flags, which `zever --help [--version]` really accepts.
	globalsLocal, _ := harvestFlags(root)
	facts.Globals = globalsLocal

	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		if cmd != root {
			cmd.InitDefaultHelpFlag()

			local, _ := harvestFlags(cmd)
			facts.Commands = append(facts.Commands, cmdFactsCommand{
				Path:       path,
				Aliases:    append([]string(nil), cmd.Aliases...),
				Hidden:     cmd.Hidden,
				Deprecated: cmd.Deprecated,
				Flags:      local,
			})
		}

		// The bare root itself is pinned through globals (no separate
		// command entry), so its own flags are not emitted twice.
		children := cmd.Commands()
		sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
		for _, child := range children {
			walk(child, joinFactsPath(path, child.Name()))
		}
	}
	walk(root, "")

	sortCommandsFacts(facts.Commands)

	return facts
}

// joinFactsPath builds the space-joined command path ("db migrate").
func joinFactsPath(parent, name string) string {
	if parent == "" {
		return name
	}

	return parent + " " + name
}

func sortFlagsFacts(flags []cmdFactsFlag) {
	sort.SliceStable(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
}

func sortCommandsFacts(cmds []cmdFactsCommand) {
	sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].Path < cmds[j].Path })
}

// factsRoot builds a fresh command tree exactly the way main does: the same
// constructor the production rootCmd initializer calls, so the fact sheet
// cannot drift between test and runtime wiring.
func factsRoot(t *testing.T) *cobra.Command {
	t.Helper()

	root := newRootCmd()
	if root == nil {
		t.Fatal("newRootCmd returned nil")
	}

	return root
}

// factsFlagsByName builds name → facts for the docs-attribution gate and
// its reference assertions.
func (f cmdFacts) factsFlagsByName() (paths map[string]bool, local map[string]map[string]bool, globals map[string]bool) {
	paths = make(map[string]bool)
	local = make(map[string]map[string]bool)
	globals = make(map[string]bool)

	for _, g := range f.Globals {
		globals[g.Name] = true
	}

	for _, c := range f.Commands {
		paths[c.Path] = true
		set := make(map[string]bool, len(c.Flags))
		for _, fl := range c.Flags {
			set[fl.Name] = true
			if fl.Shorthand != "" {
				set[fl.Shorthand] = true
			}
		}
		local[c.Path] = set
	}

	return paths, local, globals
}

// factsHasFlag reports whether path (or "zever" for the bare root) accepts
// the flag name or shorthand; help is injected on every command by cobra.
func (f cmdFacts) factsHasFlag(path, name string) bool {
	_, localFlags, globals := f.factsFlagsByName()

	if globals[name] || name == "help" {
		return true
	}

	return localFlags[path][name]
}

// TestCmdFactsGolden harvests the CLI surface and pins it against
// testdata/cmd_facts.json: with -update it rewrites the golden; without it
// the document must match byte-for-byte.
func TestCmdFactsGolden(t *testing.T) {
	facts := harvestCmdFacts(factsRoot(t))

	// Sanity floors: the harvest must see far more than a token tree.
	if len(facts.Commands) < 20 {
		t.Fatalf("harvest saw %d commands; the walk looks broken", len(facts.Commands))
	}

	if len(facts.Globals) < 5 {
		t.Fatalf("harvest saw %d global flags, want >= 5", len(facts.Globals))
	}

	// Hidden spellings and deprecated aliases must be visible to the
	// harvest exactly because the walk filters nothing: this is the hole
	// the golden closes.
	byPath := make(map[string]cmdFactsCommand, len(facts.Commands))
	for _, c := range facts.Commands {
		byPath[c.Path] = c
	}

	newCmd, ok := byPath["new"]
	if !ok {
		t.Fatal(`harvest missing the new command`)
	}

	if !hasFlagName(newCmd.Flags, "profile") {
		t.Errorf(`new facts miss --profile; the harvest must see every registered flag, deprecated aliases included`)
	}

	for _, hiddenPath := range []string{"queue:work", "schedule:run"} {
		c, ok := byPath[hiddenPath]
		if !ok {
			t.Fatalf("harvest missing %q: hidden commands must be walked", hiddenPath)
		}

		if !c.Hidden {
			t.Errorf("%q was expected hidden in the facts; got hidden=false", hiddenPath)
		}
	}

	if c, ok := byPath["check-boundaries"]; !ok || !containsAlias(c.Aliases, "check:boundaries") {
		t.Errorf(`check-boundaries facts must carry the check:boundaries alias`)
	}

	got, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		t.Fatalf("marshal facts: %v", err)
	}

	got = append(got, '\n')

	golden := filepath.Join("testdata", "cmd_facts.json")

	if updateCmdFactsRequested() {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}

		t.Logf("rewrote %s (%d bytes, %d commands, %d globals)", golden, len(got), len(facts.Commands), len(facts.Globals))

		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create it): %v", golden, err)
	}

	if string(got) != string(want) {
		t.Errorf("cmd facts golden drifted against %s: run with -update and review the diff", golden)

		if diff := factsJSONMismatch(string(want), string(got)); diff != "" {
			t.Errorf("%s", diff)
		}
	}
}

// hasFlagName reports whether the flag list carries the named flag.
func hasFlagName(flags []cmdFactsFlag, name string) bool {
	for _, f := range flags {
		if f.Name == name {
			return true
		}
	}

	return false
}

// containsAlias reports whether aliases carries the given alias spelling.
func containsAlias(aliases []string, alias string) bool {
	for _, a := range aliases {
		if a == alias {
			return true
		}
	}

	return false
}

// factsJSONMismatch renders the first divergent line pair for a readable
// failure (full detail lives in the golden diff itself).
func factsJSONMismatch(want, got string) string {
	wantLines := splitJSONLines(want)
	gotLines := splitJSONLines(got)

	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("first drift at golden line %d\n  want: %s\n  got:  %s", i+1, wantLines[i], gotLines[i])
		}
	}

	return ""
}

// splitJSONLines splits a snapshot into lines for the mismatch renderer.
func splitJSONLines(s string) []string {
	out := []string{}
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		out = append(out, s[start:])
	}

	return out
}
