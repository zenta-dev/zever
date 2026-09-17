package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunExtractFlagsAfterPositional(t *testing.T) {
	// Verify both orders parse: `zever extract shop --out ./shop-service` vs `zever extract --out ./shop-service shop`
	for _, tc := range []struct {
		name string
		args func(outDir string) []string
	}{
		{"module first flags after", func(out string) []string { return []string{"billing", "--out", out} }},
		{"flags first module after", func(out string) []string { return []string{"--out", out, "billing"} }},
		{"module first longform equals", func(out string) []string { return []string{"billing", "--out=" + out} }},
		{"interleaved with module path", func(out string) []string {
			return []string{"billing", "--module", "example.com/billing-service", "--out", out}
		}},
		{"flags before module with module flag", func(out string) []string {
			return []string{"--module", "example.com/billing-service", "--out", out, "billing"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := setupExtractProject(t)
			outDir := filepath.Join(dir, "billing-service-"+strings.ReplaceAll(tc.name, " ", "-"))
			if err := runExtract(tc.args(outDir)); err != nil {
				t.Fatalf("runExtract %q: %v", tc.name, err)
			}
			if _, err := os.Stat(filepath.Join(outDir, "go.mod")); err != nil {
				t.Fatalf("expected go.mod to exist for %q: %v", tc.name, err)
			}
			// Ensure the correct module was extracted (billing-service should contain billing module files)
			if _, err := os.Stat(filepath.Join(outDir, "schema", "billing.zen")); err != nil {
				t.Fatalf("expected billing.zen in %q: %v", tc.name, err)
			}
		})
	}
}

func TestRunExtractWithForceFlagAfterPositional(t *testing.T) {
	dir, _ := setupExtractProject(t)
	outDir := filepath.Join(dir, "custom-out")
	// Module first, --force after --out
	if err := runExtract([]string{"billing", "--out", outDir}); err != nil {
		t.Fatalf("first extract: %v", err)
	}
	const handwritten = "package main\n\nfunc main() {}\n"
	path := filepath.Join(outDir, "cmd", "server", "main.go")
	if err := os.WriteFile(path, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	// --force after positional should be parsed and allow overwrite
	if err := runExtract([]string{"billing", "--out", outDir, "--force"}); err != nil {
		t.Fatalf("runExtract with --force after positional: %v", err)
	}
	if got := readFile(t, path); got == handwritten {
		t.Fatalf("--force after positional did not overwrite")
	}
}

func TestFlexibleParseExtractFlags(t *testing.T) {
	// Unit-level: flexibleParse with extract-style flags
	fs := newTestFlagSetForExtract()
	args := []string{"mymodule", "--out", "./shop-service"}
	got, err := flexibleParse(fs, args)
	if err != nil {
		t.Fatalf("flexibleParse: %v", err)
	}
	if len(got) != 1 || got[0] != "mymodule" {
		t.Fatalf("pos = %v, want [mymodule]", got)
	}
	if fs.Lookup("out").Value.String() != "./shop-service" {
		t.Fatalf("out = %q, want ./shop-service", fs.Lookup("out").Value.String())
	}
	// Reversed
	fs2 := newTestFlagSetForExtract()
	args2 := []string{"--out", "./shop-service", "mymodule"}
	got2, err := flexibleParse(fs2, args2)
	if err != nil {
		t.Fatalf("flexibleParse reversed: %v", err)
	}
	if len(got2) != 1 || got2[0] != "mymodule" {
		t.Fatalf("pos reversed = %v, want [mymodule]", got2)
	}
	if fs2.Lookup("out").Value.String() != "./shop-service" {
		t.Fatalf("out reversed = %q, want ./shop-service", fs2.Lookup("out").Value.String())
	}
}

func newTestFlagSetForExtract() *flag.FlagSet {
	// helper to avoid import cycle - inline to avoid duplicate definition with args_test.go
	// Use flag directly
	return newExtractFlagSet()
}

// newExtractFlagSet mirrors runExtract flags.
func newExtractFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("extract-test", flag.ContinueOnError)
	fs.String("out", "", "")
	fs.String("module", "", "")
	fs.Bool("force", false, "")
	return fs
}
