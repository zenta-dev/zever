package main

import (
	"strings"
	"testing"
)

// TestAllTopLevelCoversCobraTree pins P3-5: the suggestion list must derive
// from the Cobra tree, so every registered top-level command (including
// docs and completion) and every alias (including the deliberate
// check:boundaries pair) is suggestible. Fails while allTopLevel is a
// hand-maintained list that omits docs and completion.
func TestAllTopLevelCoversCobraTree(t *testing.T) {
	_ = t.Context()

	root := newRootCmd()

	have := map[string]bool{}
	for _, n := range allTopLevel() {
		have[n] = true
	}

	for _, c := range root.Commands() {
		for _, n := range append([]string{c.Name()}, c.Aliases...) {
			if !have[n] {
				t.Errorf("command %q not in suggestion list", n)
			}
		}
	}

	for _, want := range []string{"docs", "completion", "check-boundaries", "check:boundaries", "help"} {
		if !have[want] {
			t.Errorf("suggestion list missing %q", want)
		}
	}

	if got := closest("docss", allTopLevel()); got != "docs" {
		t.Errorf("closest(docss) = %q, want %q", got, "docs")
	}

	if got := closest("completionn", allTopLevel()); got != "completion" {
		t.Errorf("closest(completionn) = %q, want %q", got, "completion")
	}

	if got := closest("check-boundarie", allTopLevel()); got != "check-boundaries" {
		t.Errorf("closest(check-boundarie) = %q, want %q", got, "check-boundaries")
	}

	if got := closest("check:boundarie", allTopLevel()); got != "check:boundaries" {
		t.Errorf("closest(check:boundarie) = %q, want %q", got, "check:boundaries")
	}
}

// TestEveryCommandAppearsInHelp pins P3-5: every registered top-level
// command must appear in both `zever --help` (Cobra) and the
// unknown-command usage text (printUsage). Fails while printUsage omits
// docs and completion.
func TestEveryCommandAppearsInHelp(t *testing.T) {
	_ = t.Context()

	prev := colorEnabled
	t.Cleanup(func() { colorEnabled = prev })
	colorEnabled = false

	root := newRootCmd()
	var names []string
	for _, c := range root.Commands() {
		if c.Name() == "help" {
			continue
		}
		names = append(names, c.Name())
	}

	usage := captureStderr(t, func() { printUsage() })

	stdout, stderr, err := executeRoot(t, "--help")
	if err != nil {
		t.Fatalf("--help = %v, want nil", err)
	}
	cobraHelp := stdout + stderr

	for _, n := range names {
		if !strings.Contains(cobraHelp, n) {
			t.Errorf("--help missing command %q", n)
		}
		if !strings.Contains(usage, n) {
			t.Errorf("unknown-command usage missing command %q", n)
		}
	}
}

// TestTopLevelHelpHeaderUnified pins P3-5: `zever --help` and the
// unknown-command usage text must share the same header line.
func TestTopLevelHelpHeaderUnified(t *testing.T) {
	_ = t.Context()

	prev := colorEnabled
	t.Cleanup(func() { colorEnabled = prev })
	colorEnabled = false

	usage := captureStderr(t, func() { printUsage() })

	stdout, stderr, err := executeRoot(t, "--help")
	if err != nil {
		t.Fatalf("--help = %v, want nil", err)
	}

	firstLine := func(s string) string {
		for _, ln := range strings.Split(s, "\n") {
			if strings.TrimSpace(ln) != "" {
				return ln
			}
		}
		return ""
	}

	if got, want := firstLine(stdout+stderr), firstLine(usage); got != want {
		t.Errorf("--help header = %q, usage header = %q, want identical", got, want)
	}
}
