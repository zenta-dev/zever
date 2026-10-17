package main

import (
	"strings"
	"testing"
)

// completeLines runs cobra's hidden __complete command and returns the
// candidate lines (descriptions stripped) plus the trailing directive line.
func completeLines(t *testing.T, args ...string) (cands []string, directive string) {
	t.Helper()

	out, _, err := executeRoot(t, append([]string{"__complete"}, args...)...)
	if err != nil {
		t.Fatalf("__complete %v: %v", args, err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	directive = lines[len(lines)-1]

	for _, l := range lines[:len(lines)-1] {
		name, _, _ := strings.Cut(l, "\t")
		cands = append(cands, name)
	}

	return cands, directive
}

func hasCand(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}

	return false
}

func TestCompletionRoutesToCobra(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{{"__complete", "x"}, {"__completeNoDesc", "x"}} {
		if !useCobra(args) {
			t.Errorf("useCobra(%v) = false, want true", args)
		}
	}
}

func TestCompleteZenFiles(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	for _, cmd := range []string{"check", "compile", "fmt", "breaking", "routes", "explain", "graph", "check-boundaries"} {
		cands, dir := completeLines(t, cmd, "")
		if !hasCand(cands, "zen") || !strings.Contains(dir, "8") { // 8 = FilterFileExt
			t.Errorf("%s: candidates %v directive %q, want zen filter", cmd, cands, dir)
		}
	}
}

func TestCompleteGenerateKinds(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	cands, _ := completeLines(t, "generate", "")
	for _, want := range allGenerate {
		if !hasCand(cands, want) {
			t.Errorf("generate kinds %v missing %q", cands, want)
		}
	}

	cands, _ = completeLines(t, "generate", "e")
	if len(cands) != 1 || cands[0] != "entity" {
		t.Errorf("generate e = %v, want [entity]", cands)
	}
}

func TestCompleteBackendFlag(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	for _, args := range [][]string{
		{"compile", "--backend", "go"},
		{"compile", "--backend=go"},
	} {
		cands, _ := completeLines(t, args...)
		if !hasCand(cands, "gogen") {
			t.Errorf("%v = %v, want gogen", args, cands)
		}
	}

	cands, _ := completeLines(t, "compile", "--backend", "gogen,at")
	if !hasCand(cands, "gogen,atlas") {
		t.Errorf("comma list = %v, want gogen,atlas", cands)
	}

	cands, _ = completeLines(t, "compile", "--backend", "gogen,go")
	if hasCand(cands, "gogen,gogen") {
		t.Errorf("already-chosen backend offered again: %v", cands)
	}
}

func TestCompleteProfileFlag(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	cands, _ := completeLines(t, "new", "app", "--profile", "")
	for _, want := range []string{"minimal", "api", "full"} {
		if !hasCand(cands, want) {
			t.Errorf("--profile candidates %v missing %q", cands, want)
		}
	}
}

func TestCompleteAdaptersFlags(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	cands, _ := completeLines(t, "new", "app", "--adapters", "ca")
	if !hasCand(cands, "cache=") {
		t.Errorf("new --adapters ca = %v, want cache=", cands)
	}

	cands, _ = completeLines(t, "new", "app", "--adapters", "cache=")
	if !hasCand(cands, "cache=memory") {
		t.Errorf("new --adapters cache= = %v, want cache=memory", cands)
	}

	cands, _ = completeLines(t, "build", "--adapters", "cache/")
	if !hasCand(cands, "cache/memory") {
		t.Errorf("build --adapters cache/ = %v, want cache/memory", cands)
	}

	cands, _ = completeLines(t, "build", "--adapters", "log,ca")
	if !hasCand(cands, "log,cache") {
		t.Errorf("build --adapters log,ca = %v, want log,cache", cands)
	}
}

func TestCompleteFlagNames(t *testing.T) {
	// Not parallel: Cobra's PersistentPreRun writes package-level mode vars.
	cands, _ := completeLines(t, "doctor", "--")
	for _, want := range []string{"--strict", "--json"} {
		if !hasCand(cands, want) {
			t.Errorf("doctor flag names %v missing %q", cands, want)
		}
	}

	for _, gone := range []string{"--config"} {
		if hasCand(cands, gone) {
			t.Errorf("doctor flag names %v still offer removed %q", cands, gone)
		}
	}
}
