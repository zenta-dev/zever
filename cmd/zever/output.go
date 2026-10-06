package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
)

// Machine-readable exit codes. The mapping preserves the existing contract:
// 0 success, 1 runtime error, 2 flag misuse. Agents branch on ExitCode
// without parsing prose.
const (
	// ExitOK reports successful completion.
	ExitOK = 0
	// ExitError reports a runtime error.
	ExitError = 1
	// ExitUsage reports flag misuse.
	ExitUsage = 2
)

// jsonMode selects JSON envelope output for machine consumption. Set from the
// global --json flag (or ZEVER_JSON) before dispatch; handlers check it via
// their configs. Human output is unchanged when false.
var jsonMode bool

// jsonExplicit reports whether --json/--json= appeared on the command line,
// so an explicit --json=false wins over ZEVER_JSON.
var jsonExplicit bool

// jsonOut is the JSON envelope sink. Tests swap it to capture output.
var jsonOut io.Writer = os.Stdout

// Envelope is the machine-readable result wrapper written in JSON mode.
type Envelope struct {
	// OK reports whether the command succeeded.
	OK bool `json:"ok"`
	// Command is the invoked subcommand, or "" when unknown.
	Command string `json:"command"`
	// ExitCode is the process exit code for this result.
	ExitCode int `json:"exitCode"`
	// Data carries command-specific structured output on success.
	Data any `json:"data,omitempty"`
	// Error carries the failure message on error.
	Error string `json:"error,omitempty"`
	// Hint carries a human-readable recovery hint on error.
	Hint string `json:"hint,omitempty"`
	// Breadcrumbs suggests follow-up commands.
	Breadcrumbs []string `json:"breadcrumbs,omitempty"`
}

// exitCodeFor maps an error to a typed exit code, mirroring main's mapping.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}

	if errors.Is(err, errFlagUsage) {
		return ExitUsage
	}

	return ExitError
}

// emitEnvelope writes env as a single JSON line to jsonOut.
func emitEnvelope(env Envelope) {
	_ = json.NewEncoder(jsonOut).Encode(env)
}

// emitSuccess writes a success envelope for command with data.
func emitSuccess(command string, data any) {
	emitEnvelope(Envelope{
		OK:          true,
		Command:     command,
		ExitCode:    ExitOK,
		Data:        data,
		Breadcrumbs: breadcrumbsFor(command),
	})
}

// emitError writes an error envelope for command.
func emitError(command string, err error) {
	emitEnvelope(Envelope{
		OK:          false,
		Command:     command,
		ExitCode:    exitCodeFor(err),
		Error:       err.Error(),
		Breadcrumbs: breadcrumbsFor(command),
	})
}

// invokedCommand returns the first non-global-flag positional in args, or ""
// when there is none. It mirrors useCobra's routing scan without executing.
func invokedCommand(args []string) string {
	for _, a := range args {
		if globalFlagToken(a) || a == "--json" {
			continue
		}

		if len(a) > 0 && a[0] == '-' {
			continue
		}

		return a
	}

	return ""
}

// breadcrumbsFor suggests follow-up commands for a subcommand.
func breadcrumbsFor(command string) []string {
	switch command {
	case "compile":
		return []string{"zever check", "zever explain", "zever extract"}
	case "check":
		return []string{"zever compile", "zever fmt"}
	case "explain":
		return []string{"zever routes", "zever compile"}
	case "doctor":
		return []string{"zever config show"}
	case "new":
		return []string{"zever generate", "zever compile"}
	case "add":
		return []string{"zever compile", "zever doctor"}
	case "extract":
		return []string{"zever compile", "zever check"}
	default:
		return nil
	}
}

// envJSON returns true if ZEVER_JSON=1/true/yes (case-insensitive).
func envJSON() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("ZEVER_JSON")))
	return v == "1" || v == "true" || v == "yes"
}

// peelJSON scans args for --json (or --json=<bool>), sets jsonMode, and
// returns the filtered args. Mirrors peelInteractive so the global flag works
// before or after the subcommand on the legacy path. An explicit --json=false
// disables JSON even when ZEVER_JSON is set.
func peelJSON(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" {
			jsonMode = true
			jsonExplicit = true
			continue
		}

		if v, ok := strings.CutPrefix(a, "--json="); ok {
			if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
				jsonMode = b
			} else {
				jsonMode = true
			}
			jsonExplicit = true
			continue
		}

		filtered = append(filtered, a)
	}

	if !jsonExplicit && envJSON() {
		jsonMode = true
	}

	return filtered
}
