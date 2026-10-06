package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	t.Parallel()

	if got := exitCodeFor(nil); got != ExitOK {
		t.Errorf("exitCodeFor(nil) = %d, want %d", got, ExitOK)
	}

	if got := exitCodeFor(errors.New("boom")); got != ExitError {
		t.Errorf("exitCodeFor(err) = %d, want %d", got, ExitError)
	}

	if got := exitCodeFor(&flagUsageError{err: errors.New("bad flag")}); got != ExitUsage {
		t.Errorf("exitCodeFor(flag usage) = %d, want %d", got, ExitUsage)
	}
}

func TestInvokedCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"bare", []string{"compile", "a.zen"}, "compile"},
		{"global before", []string{"--json", "compile"}, "compile"},
		{"global after", []string{"compile", "--json"}, "compile"},
		{"short", []string{"-q", "doctor"}, "doctor"},
		{"empty", nil, ""},
		{"flags only", []string{"--json"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := invokedCommand(tc.args); got != tc.want {
				t.Errorf("invokedCommand(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestPeelJSON(t *testing.T) {
	oldMode, oldExplicit := jsonMode, jsonExplicit
	t.Cleanup(func() { jsonMode, jsonExplicit = oldMode, oldExplicit })

	filtered := peelJSON([]string{"compile", "--json", "a.zen"})
	if !jsonMode {
		t.Fatal("jsonMode not set by --json")
	}

	for _, a := range filtered {
		if a == "--json" {
			t.Fatalf("--json not stripped: %v", filtered)
		}
	}
}

func TestPeelJSONEqualsForm(t *testing.T) {
	oldMode, oldExplicit := jsonMode, jsonExplicit
	t.Cleanup(func() { jsonMode, jsonExplicit = oldMode, oldExplicit })

	jsonMode = false
	jsonExplicit = false
	peelJSON([]string{"--json=false"})

	if jsonMode {
		t.Fatal("jsonMode set by --json=false")
	}

	if !jsonExplicit {
		t.Fatal("jsonExplicit not set by --json=false")
	}
}

func TestPeelJSONEqualsTrueForm(t *testing.T) {
	oldMode, oldExplicit := jsonMode, jsonExplicit
	t.Cleanup(func() { jsonMode, jsonExplicit = oldMode, oldExplicit })

	jsonMode = false
	jsonExplicit = false
	peelJSON([]string{"--json=True"})

	if !jsonMode {
		t.Fatal("jsonMode not set by --json=True")
	}
}

func TestEmitErrorEnvelope(t *testing.T) {
	var buf bytes.Buffer
	old := jsonOut
	jsonOut = &buf
	t.Cleanup(func() { jsonOut = old })

	emitError("compile", errors.New("boom"))

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v", err)
	}

	if env.OK || env.Command != "compile" || env.ExitCode != ExitError || env.Error != "boom" {
		t.Fatalf("envelope = %+v, want error envelope for compile", env)
	}
}

func TestEmitSuccessEnvelope(t *testing.T) {
	var buf bytes.Buffer
	old := jsonOut
	jsonOut = &buf
	t.Cleanup(func() { jsonOut = old })

	emitSuccess("check", map[string]any{"valid": true})

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v", err)
	}

	if !env.OK || env.Command != "check" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want success envelope for check", env)
	}
}

func TestAgentHelpRequested(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"help agent", []string{"--help", "--agent"}, true},
		{"agent help", []string{"--agent", "--help"}, true},
		{"short", []string{"-h", "--agent"}, true},
		{"help word", []string{"help", "--agent"}, true},
		{"subcommand help", []string{"compile", "-h", "--agent"}, false},
		{"subcommand only", []string{"compile", "--agent"}, false},
		{"help only", []string{"--help"}, false},
		{"agent only", []string{"--agent"}, false},
		{"empty", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := agentHelpRequested(tc.args); got != tc.want {
				t.Errorf("agentHelpRequested(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestPrintAgentHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printAgentHelp(&buf)

	var catalog struct {
		Commands []struct {
			Name  string `json:"name"`
			Short string `json:"short"`
		} `json:"commands"`
		GlobalFlags []struct {
			Name string `json:"name"`
		} `json:"globalFlags"`
		ExitCodes []struct {
			Code int `json:"code"`
		} `json:"exitCodes"`
	}

	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &catalog); err != nil {
		t.Fatalf("catalog is not JSON: %v", err)
	}

	if len(catalog.Commands) == 0 {
		t.Fatal("no commands in catalog")
	}

	var sawCompile, sawJSON bool

	for _, c := range catalog.Commands {
		if c.Name == "compile" && c.Short != "" {
			sawCompile = true
		}
	}

	for _, f := range catalog.GlobalFlags {
		if f.Name == "json" {
			sawJSON = true
		}
	}

	if !sawCompile {
		t.Error("catalog missing compile command")
	}

	if !sawJSON {
		t.Error("catalog missing json global flag")
	}

	if len(catalog.ExitCodes) != 3 {
		t.Errorf("exit codes = %d, want 3", len(catalog.ExitCodes))
	}
}
